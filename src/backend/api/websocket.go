package api

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"net/url"
	"sort"
	"strings"
	"sync"
	"typonamer/config"
	"typonamer/lookup/lookuper"
	"typonamer/scheduler"
	"typonamer/typo"
	"typonamer/utils"
)

type clientInfo struct {
	mu           sync.Mutex
	Kws          *socketio.Websocket
	WebCheckTask *scheduler.WebCheck
	RegisterTask *scheduler.RegisterTask
	Token        string
}
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
	Domains      []string `json:"domains"`
	Confirmed    bool     `json:"confirmed"`
}
type TypoCheck struct {
	Domain    string   `json:"domain"`
	TypoType  []string `json:"typoType"`
	CcTlds    []string `json:"ccTlds"`
	QueryType string   `json:"queryType"`
}

var clientsMu sync.Mutex
var clients = map[string]*clientInfo{}

func init() {
	socketio.On(socketio.EventDisconnect, onDisconnect)
	socketio.On(socketio.EventMessage, onMessage)
}
func emit(k *socketio.Websocket, event string, data interface{}) {
	b, _ := json.Marshal(map[string]interface{}{"event": event, "data": data})
	k.Emit(b, socketio.TextMessage)
}
func onDisconnect(ep *socketio.EventPayload) {
	clientsMu.Lock()
	c := clients[ep.Kws.UUID]
	delete(clients, ep.Kws.UUID)
	clientsMu.Unlock()
	if c != nil {
		c.mu.Lock()
		if c.WebCheckTask != nil {
			c.WebCheckTask.Stop()
		}
		if c.RegisterTask != nil {
			c.RegisterTask.Stop()
		}
		c.mu.Unlock()
	}
	scheduler.BulkCheckRemoveKws(ep.Kws)
}
func onMessage(ep *socketio.EventPayload) {
	if len(ep.Data) > 4*1024*1024 {
		emit(ep.Kws, "webCheckError", "请求过大")
		return
	}
	var m MessageObject
	if json.Unmarshal(ep.Data, &m) != nil {
		emit(ep.Kws, "webCheckError", "无效 JSON")
		return
	}
	clientsMu.Lock()
	c := clients[ep.Kws.UUID]
	clientsMu.Unlock()
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if m.Event == "ping" {
		ep.Kws.Emit([]byte(`{"event":"pong"}`), socketio.TextMessage)
		return
	}
	if m.Event == "adminAuth" {
		var token string
		if json.Unmarshal(m.Data, &token) != nil {
			return
		}
		c.Token = token
		if validAdminToken(token) {
			scheduler.BulkCheckAddKws(ep.Kws, func() bool { return validAdminToken(token) })
		} else {
			scheduler.BulkCheckRemoveKws(ep.Kws)
		}
		return
	}
	admin := c.Token != "" && validAdminToken(c.Token)
	if !admin {
		scheduler.BulkCheckRemoveKws(ep.Kws)
	}
	if m.Event != "webCheck" && m.Event != "webCheckStop" && !admin {
		event := "webCheckError"
		switch m.Event {
		case "register", "typoCheck":
			event = m.Event + "Error"
		case "bulkCheckStart", "bulkCheckPause", "bulkCheckResume", "bulkCheckCancel", "bulkCheckClear", "bulkRecheckErrorDomains":
			event = "bulkCheckError"
		}
		emit(ep.Kws, event, "需要管理员登录")
		return
	}
	switch m.Event {
	case "webCheck":
		var req WebCheck
		if json.Unmarshal(m.Data, &req) != nil {
			emit(ep.Kws, "webCheckError", "无效查询")
			return
		}
		runWeb(c, req.Domains, req.QueryType, "webCheck", !admin)
	case "webCheckStop":
		if c.WebCheckTask != nil {
			c.WebCheckTask.Stop()
			c.WebCheckTask = nil
		}
		emit(ep.Kws, "webCheckStopped", nil)
	case "typoCheck":
		var req TypoCheck
		if json.Unmarshal(m.Data, &req) != nil {
			emit(ep.Kws, "typoCheckError", "无效查询")
			return
		}
		generated, err := typo.Generate(typo.Request{Domain: req.Domain, Types: req.TypoType, TLDs: req.CcTlds})
		if err != nil {
			emit(ep.Kws, "typoCheckError", err.Error())
			return
		}
		if len(generated.Domains) == 0 {
			emit(ep.Kws, "typoCheckError", "所选类型和后缀没有生成与原域名不同的变体，请增加 Typo 类型或后缀")
			return
		}
		if len(generated.Domains) > config.GetConfig().WebCheckDomainLimit {
			emit(ep.Kws, "typoCheckError", "生成数量超过网页查询上限，请减少类型或通过 API 分批查询")
			return
		}
		if !lookuper.ValidMode(req.QueryType) {
			emit(ep.Kws, "typoCheckError", "无效查询类型")
			return
		}
		kinds := make([]string, 0, len(generated.Groups))
		for kind := range generated.Groups {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			emit(ep.Kws, "typoResult", map[string]interface{}{"typoType": kind, "domains": generated.Groups[kind]})
		}
		runWeb(c, generated.Domains, req.QueryType, "typoCheck", false)
	case "bulkCheckStart":
		var req struct {
			QueryType string `json:"queryType"`
		}
		if json.Unmarshal(m.Data, &req) != nil || !lookuper.ValidMode(req.QueryType) {
			emit(ep.Kws, "bulkCheckError", "无效查询类型")
			return
		}
		if err := scheduler.SetBulkCheckQueryType(req.QueryType); err != nil {
			emit(ep.Kws, "bulkCheckError", "保存查询类型失败")
			return
		}
		go scheduler.CreateBulkCheckTask()
	case "bulkCheckPause":
		scheduler.PauseBulkCheckTask()
	case "bulkCheckResume":
		scheduler.ResumeBulkCheckTask()
	case "bulkCheckCancel":
		scheduler.CancelBulkCheckTask()
	case "bulkCheckClear":
		scheduler.ClearBulkCheckTask()
	case "bulkRecheckErrorDomains":
		scheduler.RecheckBulkCheckErrorDomains()
	case "register":
		var req Register
		if json.Unmarshal(m.Data, &req) != nil || !req.Confirmed {
			emit(ep.Kws, "registerError", "注册需要明确确认")
			return
		}
		domains, err := validDomains(req.Domains)
		if err != nil {
			emit(ep.Kws, "registerError", err.Error())
			return
		}
		if c.RegisterTask != nil {
			c.RegisterTask.Stop()
		}
		c.RegisterTask = scheduler.NewRegisterTask(ep.Kws.UUID, ep.Kws)
		c.RegisterTask.SetDomains(domains)
		token := c.Token
		go c.RegisterTask.Run(req.RegisterType, func() bool { return validAdminToken(token) })
	default:
		emit(ep.Kws, "webCheckError", "未知操作")
	}
}
func validDomains(input []string) ([]string, error) {
	if len(input) == 0 || len(input) > config.GetConfig().WebCheckDomainLimit {
		return nil, fiber.NewError(400, "域名数量超过限制或为空")
	}
	seen := map[string]bool{}
	out := []string{}
	for i, d := range input {
		n, e := utils.TrimAndGetMainDomain(d)
		if e != nil {
			return nil, fiber.NewError(400, fmt.Sprintf("第 %d 项域名格式错误，请输入纯域名，每行一个", i+1))
		}
		if !seen[n] {
			out = append(out, n)
			seen[n] = true
		}
	}
	return out, nil
}
func runWeb(c *clientInfo, input []string, mode, kind string, public bool) {
	errEvent := "webCheckError"
	if kind == "typoCheck" {
		errEvent = "typoCheckError"
	}
	domains, err := validDomains(input)
	if err != nil {
		emit(c.Kws, errEvent, err.Error())
		return
	}
	if !lookuper.ValidMode(mode) {
		emit(c.Kws, errEvent, "无效查询类型")
		return
	}
	if c.WebCheckTask != nil {
		c.WebCheckTask.Stop()
	}
	if kind == "webCheck" {
		emit(c.Kws, "webCheckDomains", domains)
	}
	c.WebCheckTask = scheduler.NewWebCheckTask(c.Kws.UUID, c.Kws)
	c.WebCheckTask.SetDomains(domains)
	go c.WebCheckTask.Run(mode, public)
}
func WebSocketUpgrade(c *fiber.Ctx) error {
	if origin := c.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, string(c.Request().Host())) {
			return c.SendStatus(403)
		}
	}
	if websocket.IsWebSocketUpgrade(c) {
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}
func WebSocketHandler(k *socketio.Websocket) {
	k.Conn.SetReadLimit(4 * 1024 * 1024)
	clientsMu.Lock()
	defer clientsMu.Unlock()
	clients[k.UUID] = &clientInfo{Kws: k}
	// Tokens arrive in the first frame, never in URL query strings or proxy access logs.
}
