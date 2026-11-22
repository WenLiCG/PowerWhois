package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"typonamer/config"
	"typonamer/internal/testredis"
	"typonamer/scheduler"

	"github.com/fasthttp/websocket"
)

func TestBrowserProtocolAndBulkDownload(t *testing.T) {
	client := testredis.Start(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("domain") == "example.com" {
			io.WriteString(w, "fixture-free")
		} else {
			io.WriteString(w, "fixture-taken")
		}
	}))
	defer provider.Close()
	t.Setenv("POWERWHOIS_ADMIN_PASSWORD", "fixture-login")
	t.Setenv("POWERWHOIS_JWT_SECRET", "fixture-signature")
	cfg := "AuthUsername: admin\nWhoisApis:\n - ApiName: fixture\n   ApiUrl: '" + provider.URL +
		"/?domain={domain}'\n   FreeText: [fixture-free]\n   TakenText: [fixture-taken]\n   ConcurrencyLimit: 1\n"
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	bulk, err := scheduler.NewBulkCheck(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	app, drain := newApp(ctx, bulk, client)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(ln)
	defer func() {
		cancel()
		stopCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = bulk.Shutdown(stopCtx)
		_ = app.ShutdownWithContext(stopCtx)
		_ = drain(stopCtx)
		ln.Close()
	}()

	base := "http://" + ln.Addr().String()
	call := func(method, path, jwt, media string, body io.Reader) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if jwt != "" {
			req.Header.Set("Authorization", "Bearer "+jwt)
		}
		if media != "" {
			req.Header.Set("Content-Type", media)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}
	code, raw := call("GET", "/api/web/setting", "", "", nil)
	if code != 200 || bytes.Contains(raw, []byte("fixture\"")) {
		t.Fatal("anonymous settings should not list private providers")
	}
	code, raw = call("POST", "/api/login", "", "application/json", strings.NewReader(`{"username":"admin","password":"fixture-login"}`))
	var login struct{ Token string }
	if code != 200 || json.Unmarshal(raw, &login) != nil || login.Token == "" {
		t.Fatal("test administrator could not sign in")
	}
	code, raw = call("GET", "/api/admin/setting", login.Token, "", nil)
	if code != 200 || bytes.Contains(raw, []byte("fixture-login")) || bytes.Contains(raw, []byte("fixture-signature")) ||
		bytes.Contains(raw, []byte(provider.URL)) {
		t.Fatal("admin settings response included a credential or provider URL")
	}

	headers := http.Header{"Origin": {base}}
	socket, _, err := websocket.DefaultDialer.DialContext(ctx, "ws://"+ln.Addr().String()+"/app/ws", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	_ = socket.SetReadDeadline(time.Now().Add(10 * time.Second))
	send := func(event string, data any) {
		t.Helper()
		if err := socket.WriteJSON(map[string]any{"event": event, "data": data}); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(event string, accept func(json.RawMessage) bool) {
		t.Helper()
		for {
			var message struct {
				Event string
				Data  json.RawMessage
			}
			if err := socket.ReadJSON(&message); err != nil {
				t.Fatalf("waiting for %s: %v", event, err)
			}
			if message.Event == event && accept(message.Data) {
				return
			}
		}
	}
	send("adminAuth", login.Token)
	wait("adminAuthResult", func(data json.RawMessage) bool { return string(data) == "true" })
	send("webCheck", map[string]any{"queryType": "fixture", "domains": []string{"https://example.com/test"}})
	wait("webCheckResult", func(data json.RawMessage) bool {
		return bytes.Contains(data, []byte(`"registerStatus":"Free"`)) && bytes.Contains(data, []byte(`"domain":"example.com"`))
	})
	var form bytes.Buffer
	mp := multipart.NewWriter(&form)
	file, err := mp.CreateFormFile("file", "domains.txt")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(file, "example.com\nexample.net\n")
	mp.Close()
	if code, _ := call("POST", "/api/admin/bulkcheckupload", login.Token, mp.FormDataContentType(), &form); code != 200 {
		t.Fatalf("bulk upload = HTTP %d", code)
	}
	send("bulkCheckStart", map[string]string{"queryType": "fixture"})
	wait("bulkCheckInfo", func(data json.RawMessage) bool {
		var info scheduler.BulkCheckStatusInfo
		return json.Unmarshal(data, &info) == nil && info.Status == "done" && info.FreeDomains == 1 && info.TakenDomains == 1
	})
	code, raw = call("GET", "/api/admin/bulkcheckresultdownload", login.Token, "", nil)
	if code != 200 || !bytes.Contains(raw, []byte("example.com,fixture,,Free")) || !bytes.Contains(raw, []byte("example.net,fixture,,Taken")) {
		t.Fatalf("unexpected bulk CSV response, status %d", code)
	}
}
