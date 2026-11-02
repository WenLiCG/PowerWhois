package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func reply(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func httpError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, ErrNotFound) {
		status = 404
	}
	if errors.Is(err, ErrConflict) {
		status = 409
	}
	reply(w, status, map[string]string{"error": err.Error()})
}
func decode(r *http.Request, v interface{}) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 65537))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra interface{}
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}
func pageLimit(r *http.Request, max int) (int, error) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return 100, nil
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 || n > max {
		return 0, fmt.Errorf("limit must be 1..%d", max)
	}
	return n, nil
}

func (s *Service) Handler(webDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Health(); err != nil {
			reply(w, 503, map[string]string{"error": err.Error()})
			return
		}
		reply(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		limit, e := pageLimit(r, 1000)
		if e != nil {
			httpError(w, e)
			return
		}
		jobs, e := s.Store.List(r.URL.Query().Get("after"), limit)
		if e != nil {
			httpError(w, e)
			return
		}
		next := ""
		if len(jobs) == limit {
			next = jobs[len(jobs)-1].ID
		}
		reply(w, 200, map[string]interface{}{"items": jobs, "next": next})
	})
	mux.HandleFunc("POST /api/v1/jobs/import", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
		if e := r.ParseMultipartForm(8 << 20); e != nil {
			httpError(w, e)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, e := r.FormFile("file")
		if e != nil {
			httpError(w, e)
			return
		}
		defer file.Close()
		source := r.FormValue("source")
		if source == "" {
			source = filepath.Base(header.Filename)
		}
		j, e := s.Store.Import(r.Context(), file, ImportOptions{Sheet: r.FormValue("sheet"), Column: r.FormValue("column"), Header: r.FormValue("header") == "true", Source: source})
		if e != nil {
			httpError(w, e)
			return
		}
		reply(w, 201, j)
	})
	mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		j, e := s.Store.Get(r.PathValue("id"))
		if e != nil {
			httpError(w, e)
			return
		}
		reply(w, 200, j)
	})
	for _, kind := range []string{"items", "seeds", "origins"} {
		mux.HandleFunc("GET /api/v1/jobs/{id}/"+kind, func(w http.ResponseWriter, r *http.Request) {
			limit, e := pageLimit(r, 1000)
			if e != nil {
				httpError(w, e)
				return
			}
			items, next, e := s.Store.Page(r.PathValue("id"), kind, r.URL.Query().Get("after"), limit)
			if e != nil {
				httpError(w, e)
				return
			}
			reply(w, 200, map[string]interface{}{"items": items, "next": next})
		})
	}
	mux.HandleFunc("POST /api/v1/jobs/{id}/generate", func(w http.ResponseWriter, r *http.Request) {
		var o GenerateOptions
		if e := decode(r, &o); e != nil {
			httpError(w, e)
			return
		}
		j, e := s.Generate(r.Context(), r.PathValue("id"), o)
		if e != nil {
			httpError(w, e)
			return
		}
		reply(w, 200, j)
	})
	for _, action := range []string{"start", "pause", "resume", "cancel"} {
		mux.HandleFunc("POST /api/v1/jobs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			j, e := s.Action(r.PathValue("id"), action)
			if e != nil {
				httpError(w, e)
				return
			}
			reply(w, 200, j)
		})
	}
	mux.HandleFunc("GET /api/v1/jobs/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		denominator := r.URL.Query().Get("denominator")
		if denominator == "" {
			denominator = "all"
		}
		f, e := os.CreateTemp("", "powerwhois-export-*.xlsx")
		if e != nil {
			httpError(w, e)
			return
		}
		defer os.Remove(f.Name())
		defer f.Close()
		if e = s.Store.Export(r.PathValue("id"), denominator, f); e != nil {
			httpError(w, e)
			return
		}
		if _, e = f.Seek(0, io.SeekStart); e != nil {
			httpError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="powerwhois-results.xlsx"`)
		_, _ = io.Copy(w, f)
	})
	if webDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(webDir)))
	} else {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, fallbackPage)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		host, _, e := net.SplitHostPort(r.Host)
		if e != nil {
			host = r.Host
		}
		if host != "localhost" {
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				reply(w, 403, map[string]string{"error": "loopback Host required"})
				return
			}
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				reply(w, 403, map[string]string{"error": "same-origin access required"})
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && s.Health() != nil {
			if r.URL.Path != "/api/v1/health" {
				reply(w, 503, map[string]string{"error": s.Health().Error()})
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

const fallbackPage = `<!doctype html><html lang="zh"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>PowerWhois 作业</title><style>body{font:16px system-ui;max-width:1100px;margin:40px auto;padding:20px;color:#17263b}button,input{padding:10px;margin:4px}pre{white-space:pre-wrap;background:#eef3f8;padding:20px}a{color:#1261a0}</style><h1>PowerWhois 本地作业</h1><p>CLI、API 和此页面读取同一持久化作业。not_found 不保证可购买。</p><form id="form"><input name="file" type="file" accept=".xlsx" required><input name="sheet" placeholder="工作表（默认第一张）"><input name="column" value="B" size="2"><label><input type="checkbox" name="header" value="true">首行是标题</label><button>导入 XLSX</button></form><p><input id="job" placeholder="作业 ID" size="36"><button id="load">查看</button><a id="export">导出 XLSX</a></p><p id="actions"></p><pre id="view"></pre><script>
const view=document.querySelector('#view'),job=document.querySelector('#job');async function request(path,options){const r=await fetch('/api/v1/'+path,options);const data=await r.json();view.textContent=JSON.stringify(data,null,2);if(!r.ok)throw Error(data.error);return data}async function load(){if(!job.value){await request('jobs');return}await request('jobs/'+job.value);document.querySelector('#export').href='/api/v1/jobs/'+job.value+'/export';}document.querySelector('#form').onsubmit=async e=>{e.preventDefault();try{const j=await request('jobs/import',{method:'POST',body:new FormData(e.target)});job.value=j.id;await load()}catch(e){view.textContent=e.message}};document.querySelector('#load').onclick=()=>load().catch(e=>view.textContent=e.message);for(const action of ['generate','start','pause','resume','cancel']){const b=document.createElement('button');b.textContent=action;b.onclick=()=>request('jobs/'+job.value+'/'+action,{method:'POST',headers:{'Content-Type':'application/json'},body:action==='generate'?JSON.stringify({maxCandidates:500000}):'{}'}).catch(e=>view.textContent=e.message);document.querySelector('#actions').append(b)}load();</script></html>`
