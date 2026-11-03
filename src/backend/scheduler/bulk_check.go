package scheduler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofiber/contrib/socketio"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
	"typonamer/config"
	"typonamer/constant"
	"typonamer/jobs"
	"typonamer/lookup/lookupinfo"
)

var Tasks *jobs.Store
var bulkMu sync.Mutex
var bulkClients = map[*socketio.Websocket]func() bool{}
var rdb *redis.Client

// Init has no import-time network access; the server owns startup and shutdown.
func Init(ctx context.Context, r *redis.Client) {
	rdb = r
	Tasks = jobs.New(r, "powerwhois:v1:")
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := getBulkCheckInfo()
				if err == nil {
					message, _ := json.Marshal(map[string]interface{}{"event": "bulkCheckInfo", "data": info})
					bulkCheckSendWsMessage(message)
				}
			}
		}
	}()
}
func BulkCheckAddKws(k *socketio.Websocket, authorized func() bool) {
	bulkMu.Lock()
	bulkClients[k] = authorized
	bulkMu.Unlock()
}
func BulkCheckRemoveKws(k *socketio.Websocket) {
	bulkMu.Lock()
	delete(bulkClients, k)
	bulkMu.Unlock()
}
func bulkCheckSendWsMessage(data []byte) {
	bulkMu.Lock()
	clients := make([]*socketio.Websocket, 0, len(bulkClients))
	for k, authorized := range bulkClients {
		if authorized() {
			clients = append(clients, k)
		} else {
			delete(bulkClients, k)
		}
	}
	bulkMu.Unlock()
	for _, k := range clients {
		k.Emit(data, socketio.TextMessage)
	}
}
func bulkError(err error) {
	data, _ := json.Marshal(map[string]interface{}{"event": "bulkCheckError", "data": err.Error()})
	bulkCheckSendWsMessage(data)
}
func CurrentTaskID() string {
	if rdb == nil {
		return ""
	}
	return rdb.Get(context.Background(), "powerwhois:v1:web:current").Val()
}
func SelectTask(id string) error {
	if _, err := Tasks.Get(context.Background(), id); err != nil {
		return err
	}
	return rdb.Set(context.Background(), "powerwhois:v1:web:current", id, 0).Err()
}
func BulkCheckAddRawDomains(content *bytes.Buffer) error {
	if id := CurrentTaskID(); id != "" {
		t, e := Tasks.Get(context.Background(), id)
		if e != nil {
			return e
		}
		if t.State == "running" || t.State == "queued" {
			return jobs.ErrConflict
		}
	}
	return rdb.Set(context.Background(), "powerwhois:v1:web:upload", content.Bytes(), 0).Err()
}
func SetBulkCheckQueryType(mode string) error {
	return rdb.Set(context.Background(), "powerwhois:v1:web:mode", mode, 0).Err()
}
func GetBulkCheckQueryType() string {
	return rdb.Get(context.Background(), "powerwhois:v1:web:mode").Val()
}
func CreateBulkCheckTask() {
	bulkMu.Lock()
	defer bulkMu.Unlock()
	ctx := context.Background()
	if id := CurrentTaskID(); id != "" {
		t, e := Tasks.Get(ctx, id)
		if e != nil {
			go bulkError(e)
			return
		}
		if t.State == "queued" || t.State == "running" {
			return
		}
	}
	raw, err := rdb.Get(ctx, "powerwhois:v1:web:upload").Bytes()
	if err != nil {
		go bulkError(err)
		return
	}
	domains := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 4096)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		line = bytes.TrimPrefix(line, []byte{0xef, 0xbb, 0xbf})
		if len(line) > 0 {
			domains = append(domains, string(line))
		}
	}
	if err = scanner.Err(); err != nil {
		go bulkError(fmt.Errorf("input contains an overlong line"))
		return
	}
	cfg := config.GetConfig()
	retries := 0
	if cfg.RetryOnTimeout {
		retries = cfg.RetryMax
	}
	t, _, err := Tasks.Create(ctx, jobs.Request{Domains: domains, Mode: GetBulkCheckQueryType(), Concurrency: cfg.BulkCheckConcurrencyLimit, Timeout: 30, Retries: retries}, "")
	if err == nil {
		err = SelectTask(t.ID)
	}
	if err != nil {
		go bulkError(err)
		return
	}
	rdb.Del(ctx, "powerwhois:v1:web:upload")
}
func controlBulk(action string) {
	_, err := Tasks.Control(context.Background(), CurrentTaskID(), action)
	if err != nil {
		bulkError(err)
	}
}
func PauseBulkCheckTask()  { controlBulk("pause") }
func ResumeBulkCheckTask() { controlBulk("resume") }
func CancelBulkCheckTask() { controlBulk("cancel") }
func ClearBulkCheckTask() {
	if id := CurrentTaskID(); id != "" {
		t, e := Tasks.Get(context.Background(), id)
		if e != nil {
			bulkError(e)
			return
		}
		if t.State == "running" || t.State == "queued" {
			bulkError(jobs.ErrConflict)
			return
		}
	}
	if err := rdb.Del(context.Background(), "powerwhois:v1:web:current", "powerwhois:v1:web:upload").Err(); err != nil {
		bulkError(err)
	}
}
func RecheckBulkCheckErrorDomains() {
	t, err := Tasks.RetryFailed(context.Background(), CurrentTaskID())
	if err == nil {
		err = SelectTask(t.ID)
	}
	if err != nil {
		bulkError(err)
	}
}

type BulkCheckStatusInfo struct {
	ID             string
	Status         string
	QueryType      string
	TotalDomains   int
	RemainDomains  int
	TakenDomains   int
	FreeDomains    int
	ErrorDomains   int
	UnknownDomains int
	TimeoutDomains int
}

func getBulkCheckInfo() (BulkCheckStatusInfo, error) {
	info := BulkCheckStatusInfo{Status: constant.BulkCheckStatusIdle}
	ctx := context.Background()
	id := CurrentTaskID()
	if id == "" {
		if rdb.Exists(ctx, "powerwhois:v1:web:upload").Val() > 0 {
			info.Status = constant.BulkCheckStatusInit
		}
		return info, nil
	}
	t, err := Tasks.Get(ctx, id)
	if err != nil {
		return info, err
	}
	info.ID = t.ID
	info.Status = t.State
	switch t.State {
	case "queued":
		info.Status = "running"
	case "completed":
		info.Status = "done"
	case "failed":
		info.Status = "error"
	}
	info.QueryType = t.Mode
	info.TotalDomains = t.Total
	info.RemainDomains = t.Total - t.Completed
	info.TakenDomains = t.Counts["registered"]
	info.FreeDomains = t.Counts["not_found"]
	info.UnknownDomains = t.Counts["unknown"]
	info.TimeoutDomains = t.Counts["timeout"]
	info.ErrorDomains = t.Counts["error"]
	return info, nil
}
func BulkResults(ctx context.Context) ([]lookupinfo.QueryResult, error) {
	id := CurrentTaskID()
	t, err := Tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []lookupinfo.QueryResult{}
	for i := 0; i < t.Total; i += 1000 {
		p, e := Tasks.Results(ctx, id, i, 1000)
		if e != nil {
			return nil, e
		}
		out = append(out, p.Items...)
	}
	return out, nil
}
