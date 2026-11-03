package scheduler

import (
	"context"
	"encoding/json"
	"github.com/gofiber/contrib/socketio"
	"sync"
	"typonamer/config"
	"typonamer/lookup/lookuper"
)

type WebCheck struct {
	UserID  string
	Kws     *socketio.Websocket
	Domains []string
	mu      sync.Mutex
	cancel  context.CancelFunc
	stopped bool
}

func NewWebCheckTask(id string, k *socketio.Websocket) *WebCheck {
	return &WebCheck{UserID: id, Kws: k}
}
func (t *WebCheck) SetDomains(d []string) { t.Domains = append([]string(nil), d...) }
func (t *WebCheck) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.cancel != nil {
		t.cancel()
	}
}
func (t *WebCheck) Run(mode string) {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	t.mu.Unlock()
	defer cancel()
	cfg := config.GetConfig()
	ch := make(chan string)
	var wg sync.WaitGroup
	retries := 0
	if cfg.RetryOnTimeout {
		retries = cfg.RetryMax
	}
	for i := 0; i < min(cfg.WebCheckConcurrencyLimit, len(t.Domains)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range ch {
				if ctx.Err() != nil {
					return
				}
				r := lookuper.Query(ctx, d, mode, 30, retries)
				if ctx.Err() != nil {
					return
				}
				data, _ := json.Marshal(map[string]interface{}{"event": "webCheckResult", "data": r})
				t.Kws.Emit(data, socketio.TextMessage)
			}
		}()
	}
feed:
	for _, d := range t.Domains {
		select {
		case <-ctx.Done():
			break feed
		case ch <- d:
		}
	}
	close(ch)
	wg.Wait()
}
