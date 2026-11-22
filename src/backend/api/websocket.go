package api

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/network"
	"typonamer/register"
	"typonamer/scheduler"
	"typonamer/typo"
	"typonamer/utils"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

type MessageObject struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type WebCheck struct {
	QueryType string   `json:"queryType"`
	Domains   []string `json:"domains"`
}

type Register struct {
	RegisterType string   `json:"registerType"`
	OperationID  string   `json:"operationId"`
	Domains      []string `json:"domains"`
}

type TypoCheck struct {
	Domain    string   `json:"domain"`
	TypoType  []string `json:"typoType"`
	CcTlds    []string `json:"ccTlds"`
	QueryType string   `json:"queryType"`
}

type webSession struct {
	ws    *websocket.Conn
	ctx   context.Context
	bulk  *scheduler.BulkCheck
	redis *redis.Client

	wg          sync.WaitGroup
	writer      sync.Mutex
	adminUntil  atomic.Int64
	checking    atomic.Bool
	registering atomic.Bool
	nextAuth    time.Time
}

func WebSocketHandler(parent context.Context, bulk *scheduler.BulkCheck, client *redis.Client) (fiber.Handler, func(context.Context) error) {
	var sessions sync.WaitGroup
	var lifecycle sync.Mutex
	stopping := false
	allowedOrigin := os.Getenv("POWERWHOIS_WEB_ORIGIN")
	connections := make(chan struct{}, 64)
	upgrade := websocket.New(func(conn *websocket.Conn) {
		select {
		case connections <- struct{}{}:
			defer func() { <-connections }()
		default:
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "server busy"), time.Now().Add(time.Second))
			return
		}
		lifecycle.Lock()
		if stopping || parent.Err() != nil {
			lifecycle.Unlock()
			return
		}
		sessions.Add(1)
		lifecycle.Unlock()
		defer sessions.Done()

		ctx, cancel := context.WithCancel(parent)
		s := &webSession{ws: conn, ctx: ctx, bulk: bulk, redis: client}
		closed := make(chan struct{})
		closeOnStop := context.AfterFunc(ctx, func() { conn.Close(); close(closed) })
		defer func() {
			if !closeOnStop() {
				<-closed
			}
			cancel()
			s.wg.Wait()
		}()

		conn.SetReadLimit(1 << 20)
		s.wg.Go(s.bulkStatus)
		for {
			conn.SetReadDeadline(time.Now().Add(75 * time.Second))
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var envelope MessageObject
			if messageType == websocket.TextMessage && json.Unmarshal(message, &envelope) == nil {
				s.handle(envelope)
			}
		}
	}, websocket.Config{HandshakeTimeout: 5 * time.Second})
	return func(c *fiber.Ctx) error {
			if !websocket.IsWebSocketUpgrade(c) {
				return fiber.ErrUpgradeRequired
			}
			if origin := c.Get("Origin"); origin != "" && origin != allowedOrigin {
				parsed, err := url.Parse(origin)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
					!strings.EqualFold(parsed.Host, string(c.Context().Request.Header.Host())) {
					return fiber.ErrForbidden
				}
			}
			return upgrade(c)
		}, func(ctx context.Context) error {
			lifecycle.Lock()
			stopping = true
			lifecycle.Unlock()
			done := make(chan struct{})
			go func() { sessions.Wait(); close(done) }()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
}

func (s *webSession) send(event string, data any) bool {
	if s.ctx.Err() != nil {
		return false
	}
	s.writer.Lock()
	defer s.writer.Unlock()
	s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := s.ws.WriteJSON(fiber.Map{"event": event, "data": data}); err != nil {
		s.ws.Close()
		return false
	}
	return true
}

func (s *webSession) isAdmin() bool {
	return time.Now().Unix() < s.adminUntil.Load()
}

func (s *webSession) adminContext() (context.Context, context.CancelFunc) {
	return context.WithDeadline(s.ctx, time.Unix(s.adminUntil.Load(), 0))
}

func (s *webSession) handle(message MessageObject) {
	switch message.Event {
	case constant.WebsocketRequestEventPing:
		s.writer.Lock()
		s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := s.ws.WriteJSON(fiber.Map{"event": constant.WebsocketResponseEventPong}); err != nil {
			s.ws.Close()
		}
		s.writer.Unlock()
	case constant.WebsocketRequestEventAdminAuth:
		if time.Now().Before(s.nextAuth) {
			return
		}
		s.nextAuth = time.Now().Add(time.Second)
		var raw string
		if json.Unmarshal(message.Data, &raw) != nil {
			s.send("adminAuthResult", false)
			return
		}
		expiry, err := tokenExpiry(raw)
		if err != nil {
			s.adminUntil.Store(0)
		} else {
			s.adminUntil.Store(expiry.Unix())
		}
		s.send("adminAuthResult", err == nil)
	case constant.WebsocketRequestEventWebCheck:
		s.webCheck(message.Data)
	case constant.WebsocketRequestEventTypoCheck:
		s.typoCheck(message.Data)
	case constant.WebsocketRequestEventRegister:
		s.register(message.Data)
	case constant.WebsocketRequestEventBulkCheckStart, constant.WebsocketRequestEventBulkCheckPause,
		constant.WebsocketRequestEventBulkCheckResume, constant.WebsocketRequestEventBulkCheckCancel,
		constant.WebsocketRequestEventBulkCheckClear, constant.WebsocketRequestEventBulkRecheckErrorDomains:
		s.bulkAction(message)
	}
}

func (s *webSession) bulkStatus() {
	if s.bulk == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	hadError := false
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			expiry := s.adminUntil.Load()
			if expiry == 0 {
				continue
			}
			if time.Now().Unix() >= expiry {
				if s.adminUntil.CompareAndSwap(expiry, 0) {
					s.send("adminAuthResult", false)
				}
				continue
			}
			ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
			info, err := s.bulk.Info(ctx)
			cancel()
			if err != nil {
				if !hadError && s.isAdmin() {
					s.send(constant.WebsocketResponseBulkCheckErrorEvent, "暂时无法读取批量任务状态")
				}
				hadError = true
				continue
			}
			hadError = false
			if s.isAdmin() && !s.send(constant.WebsocketResponseBulkCheckInfoEvent, info) {
				return
			}
		}
	}
}

func (s *webSession) bulkAction(message MessageObject) {
	if !s.isAdmin() || s.bulk == nil {
		s.send(constant.WebsocketResponseBulkCheckErrorEvent, "请先登录且确认批量服务可用")
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	var err error
	switch message.Event {
	case constant.WebsocketRequestEventBulkCheckStart:
		var data WebCheck
		if json.Unmarshal(message.Data, &data) != nil {
			s.send(constant.WebsocketResponseBulkCheckErrorEvent, "批量任务参数格式错误")
			return
		}
		err = s.bulk.Start(ctx, data.QueryType)
	case constant.WebsocketRequestEventBulkCheckPause:
		err = s.bulk.Stop(ctx, constant.BulkCheckStatusPaused)
	case constant.WebsocketRequestEventBulkCheckResume:
		err = s.bulk.Resume(ctx)
	case constant.WebsocketRequestEventBulkCheckCancel:
		err = s.bulk.Stop(ctx, constant.BulkCheckStatusCanceled)
	case constant.WebsocketRequestEventBulkCheckClear:
		err = s.bulk.Clear(ctx)
	case constant.WebsocketRequestEventBulkRecheckErrorDomains:
		err = s.bulk.Recheck(ctx)
	}
	if err != nil {
		s.send(constant.WebsocketResponseBulkCheckErrorEvent, "无法执行此操作，请确认当前任务状态或检查服务日志")
	}
}

func (s *webSession) webCheck(data json.RawMessage) {
	const errEvent = constant.WebsocketResponseWebCheckErrorEvent
	var req WebCheck
	cfg, admin := config.GetConfig(), s.isAdmin()
	if json.Unmarshal(data, &req) != nil || len(req.Domains) > cfg.WebCheckDomainLimit || !lookuper.ValidMode(req.QueryType, admin, cfg) {
		s.send(errEvent, "查询参数无效、超过单批限制，或当前身份无法使用该接口")
		return
	}
	domains := make([]string, 0, len(req.Domains))
	seen := make(map[string]bool)
	for _, input := range req.Domains {
		name, err := utils.TrimAndGetMainDomain(input)
		if err == nil && !seen[name] {
			domains = append(domains, name)
			seen[name] = true
		}
	}
	if len(domains) == 0 {
		s.send(errEvent, "没有有效域名")
		return
	}
	s.startCheck(cfg, admin, req.QueryType, domains, errEvent, func() {
		s.send(constant.WebsocketResponseEventWebCheckDomains, domains)
	})
}

func (s *webSession) typoCheck(data json.RawMessage) {
	const errEvent = constant.WebsocketResponseTypoCheckErrorEvent
	var req TypoCheck
	cfg := config.GetConfig()
	if !s.isAdmin() || json.Unmarshal(data, &req) != nil || !lookuper.ValidMode(req.QueryType, true, cfg) {
		s.send(errEvent, "请检查 Typo 查询参数及登录状态")
		return
	}
	groups, domains, err := typo.Generate(req.Domain, req.TypoType, req.CcTlds, cfg.WebCheckDomainLimit)
	if err != nil {
		s.send(errEvent, err.Error())
		return
	}
	s.startCheck(cfg, true, req.QueryType, domains, errEvent, func() {
		for _, group := range groups {
			s.send(constant.WebsocketResponseEventTypoResult, fiber.Map{"typoType": group.Kind, "domains": group.Domains})
		}
	})
}

func (s *webSession) startCheck(cfg config.Config, admin bool, mode string, domains []string, errorEvent string, announce func()) {
	if !s.checking.CompareAndSwap(false, true) {
		s.send(errorEvent, "当前连接已有查询正在运行")
		return
	}
	ctx := s.ctx
	cancel := func() {}
	if admin {
		ctx, cancel = s.adminContext()
	}
	announce()
	s.wg.Go(func() {
		defer cancel()
		runDomains(ctx, domains, cfg.WebCheckConcurrencyLimit, func(domain string) {
			release, err := network.Acquire(ctx, "web", cfg.WebCheckConcurrencyLimit)
			if err != nil {
				return
			}
			defer release()
			result := lookuper.Query(ctx, domain, mode, admin, cfg)
			if ctx.Err() == nil {
				s.send(constant.WebsocketResponseEventWebCheckResult, result)
			}
		})
		s.checking.Store(false)
		s.send("webCheckDone", ctx.Err() != nil)
	})
}

func (s *webSession) register(data json.RawMessage) {
	const errEvent = constant.WebsocketResponseRegisterErrorEvent
	var req Register
	cfg := config.GetConfig()
	if !s.isAdmin() || s.redis == nil || json.Unmarshal(data, &req) != nil || !register.ValidOperationID(req.OperationID) ||
		len(req.Domains) == 0 || len(req.Domains) > cfg.WebCheckDomainLimit {
		s.send(errEvent, "注册参数无效、数量超限或会话不可用")
		return
	}
	provider, err := register.FindProvider(cfg, req.RegisterType)
	if err != nil {
		s.send(errEvent, err.Error())
		return
	}
	domains := make([]string, 0, len(req.Domains))
	seen := make(map[string]bool)
	for _, input := range req.Domains {
		domain, err := register.CanonicalTarget(input)
		if err != nil {
			s.send(errEvent, "注册列表包含无效域名")
			return
		}
		if !seen[domain] {
			domains = append(domains, domain)
			seen[domain] = true
		}
	}
	if !s.registering.CompareAndSwap(false, true) {
		s.send(errEvent, "当前连接已有注册正在运行")
		return
	}
	ctx, cancel := s.adminContext()
	s.wg.Go(func() {
		defer cancel()
		runDomains(ctx, domains, provider.ConcurrencyLimit, func(domain string) {
			info := register.Register(ctx, s.redis, domain, req.OperationID, provider)
			if s.isAdmin() {
				s.send(constant.WebsocketResponseEventRegisterResult, info)
			}
		})
		s.registering.Store(false)
		s.send("registerDone", nil)
	})
}

func runDomains(ctx context.Context, domains []string, workers int, work func(string)) {
	ch := make(chan string)
	var wg sync.WaitGroup
	for range min(workers, len(domains)) {
		wg.Go(func() {
			for name := range ch {
				if ctx.Err() != nil {
					return
				}
				work(name)
			}
		})
	}
send:
	for _, name := range domains {
		select {
		case ch <- name:
		case <-ctx.Done():
			break send
		}
	}
	close(ch)
	wg.Wait()
}
