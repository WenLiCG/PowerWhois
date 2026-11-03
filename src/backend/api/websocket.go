package api

import (
	"encoding/json"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/scheduler"
	"typonamer/typo"
	"typonamer/utils"

	"github.com/bytedance/sonic"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type clientInfo struct {
	Kws          *socketio.Websocket
	WebCheckTask *scheduler.WebCheck
	RegisterTask *scheduler.RegisterTask
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
}

type BulkCheckStart struct {
	QueryType string `json:"queryType"`
}

type TypoCheck struct {
	Domain    string   `json:"domain"`
	TypoType  []string `json:"typoType"`
	CcTlds    []string `json:"ccTlds"`
	QueryType string   `json:"queryType"`
}

// -----------------------------------------------

var clients = make(map[string]clientInfo)

func init() {
	// Register all websocket event handlers

	socketio.On(socketio.EventConnect, onConnect)
	socketio.On(socketio.EventDisconnect, onDisconnect)
	socketio.On(socketio.EventMessage, onMessage)

	socketio.On(constant.WebsocketRequestEventPing, onPing)
	socketio.On(constant.WebsocketRequestEventAdminAuth, onAdminAuth)

	socketio.On(constant.WebsocketRequestEventBulkCheckStart, onBulkCheckStart)
	socketio.On(constant.WebsocketRequestEventBulkCheckPause, onBulkCheckPause)
	socketio.On(constant.WebsocketRequestEventBulkCheckResume, onBulkCheckResume)
	socketio.On(constant.WebsocketRequestEventBulkCheckCancel, onBulkCheckCancel)
	socketio.On(constant.WebsocketRequestEventBulkCheckClear, onBulkCheckClear)
	socketio.On(constant.WebsocketRequestEventBulkRecheckErrorDomains, onBulkRecheckErrorDomains)

	socketio.On(constant.WebsocketRequestEventWebCheck, onWebCheck)

	socketio.On(constant.WebsocketRequestEventTypoCheck, onTypoCheck)

	socketio.On(constant.WebsocketRequestEventRegister, onRegister)
}

func onConnect(ep *socketio.EventPayload) {
	// onConnect handles the connect event for websocket. It will be called when a user first connects to the websocket.
	// It will check if the user is an admin or a public user, and log the event.
	//
	// The user is an admin if the attribute isAdmin is true. Otherwise, it is a public user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user connected. UUID: %s", ep.Kws.UUID)
	} else {
		log.Infof("Public user connected. UUID: %s", ep.Kws.UUID)
	}
}

func onDisconnect(ep *socketio.EventPayload) {
	// onDisconnect handles the disconnect event for websocket. It will be called when a user disconnects from the websocket.
	// It will log the event, and remove the user from the bulk check task and web check task if the user is an admin or a public user.
	//
	// If the user is an admin, it will log an info message, and remove the user from the bulk check task.
	// If the user is a public user, it will log an info message, and stop the web check task for the user.
	// Finally, it will delete the user from the clients map.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user disconnected. UUID: %s", ep.Kws.UUID)
		scheduler.BulkCheckRemoveKws(ep.Kws)
	} else {
		log.Infof("Public user disconnected. UUID: %s", ep.Kws.UUID)
	}

	// If the user has a web check task, stop it.
	if clientInfo, ok := clients[ep.Kws.UUID]; ok {
		if clientInfo.WebCheckTask != nil {
			log.Infof("Stop web check task for user %s", ep.Kws.UUID)
			clientInfo.WebCheckTask.Stop()
		}

		if clientInfo.RegisterTask != nil {
			log.Infof("Stop register task for user %s", ep.Kws.UUID)
			clientInfo.RegisterTask.Stop()
		}
	}
	// Delete the user from the clients map.
	delete(clients, ep.Kws.UUID)
}

func onMessage(ep *socketio.EventPayload) {
	// onMessage handles the message event for websocket. It will be called when a user sends a message to the websocket.
	// It will log the event, unmarshal the message into a MessageObject, and fire the event with the data if the event is not empty.
	// If the unmarshalling fails, or the event is empty, it will log an error.

	log.Debugf("New message from user %s is: %s", ep.Kws.UUID, string(ep.Data))

	message := MessageObject{}
	err := sonic.Unmarshal(ep.Data, &message)
	if err != nil {
		// If the unmarshalling fails, log an error.
		log.Warnf("Error unmarshalling websocket message from user %s: %s", ep.Kws.UUID, err)
		return
	}

	// If the event is not empty, fire the event with the data.
	if message.Event != "" {
		ep.Kws.Fire(message.Event, []byte(message.Data))
	} else {
		// If the event is empty, log an error.
		log.Errorf("Invalid websocket message from user %s", ep.Kws.UUID)
	}
}

func onPing(ep *socketio.EventPayload) {
	// onPing handles the ping event for websocket. It will be called when a user sends a ping to the websocket.
	// It will log the event, and emit a pong message back to the user.
	log.Debugf("New ping message from user %s", ep.Kws.UUID)
	// Emit a pong message back to the user.
	ep.Kws.Emit([]byte(convertor.ToString(fiber.Map{"event": constant.WebsocketResponseEventPong})), socketio.TextMessage)
}

func onAdminAuth(ep *socketio.EventPayload) {
	// onAdminAuth handles the admin auth message for websocket. It will be called when a user sends an admin auth message to the websocket.
	// It will log the event, trim the token, and validate the token. If the token is invalid, or the validation fails, it will log an error.
	// If the token is valid, it will set the isAdmin flag to true, and add the user to the bulk check task.

	log.Infof("Admin auth message from user %s", ep.Kws.UUID)
	token := strutil.Trim(string(ep.Data), "\"")
	log.Debug("Admin auth token is: " + token)
	isValid, err := ValidateToken(token)
	if err != nil {
		// If the validation fails, log an error.
		log.Warnf("Error validating token from user %s: %s", ep.Kws.UUID, err)
	} else if !isValid {
		// If the token is invalid, log an error.
		log.Warnf("Invalid token from user %s", ep.Kws.UUID)
	} else {
		// If the token is valid, set the isAdmin flag to true, and add the user to the bulk check task.
		ep.Kws.SetAttribute("isAdmin", true)
		scheduler.BulkCheckAddKws(ep.Kws)
		log.Infof("Valid token from user %s, and now as admin", ep.Kws.UUID)
	}
}

func onBulkCheckStart(ep *socketio.EventPayload) {
	// onBulkCheckStart handles the bulk check start event for websocket. It will be called when an admin user sends a bulk check start message to the websocket.
	// It will log the event, unmarshal the message, set the query type, and start the bulk check task.
	// If the validation fails, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s start bulk check task", ep.Kws.UUID)

		// Unmarshal the message into a BulkCheckStart structure.
		startMessage := BulkCheckStart{}
		err := sonic.Unmarshal(ep.Data, &startMessage)
		if err != nil {
			// If the unmarshalling fails, log an error, and emit an error message back to the user.
			log.Warnf("Error unmarshalling websocket bulk check start message from user %s: %s", ep.Kws.UUID, err)
			responseError := map[string]interface{}{
				"event": constant.WebsocketResponseBulkCheckErrorEvent,
				"data":  "出现错误: " + err.Error(),
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
			return
		}

		// Set the query type of the bulk check task.
		err = scheduler.SetBulkCheckQueryType(startMessage.QueryType)
		if err == nil {
			log.Debugf("Admin user UUID %s set bulk check query type to %s", ep.Kws.UUID, startMessage.QueryType)
			// Start the bulk check task.
			go scheduler.CreateBulkCheckTask()
		} else {
			// If the setting fails, log an error, and emit an error message back to the user.
			log.Warnf("Error setting bulk check query type for user %s: %s", ep.Kws.UUID, err)
			responseError := map[string]interface{}{
				"event": constant.WebsocketResponseBulkCheckErrorEvent,
				"data":  "出现错误: " + err.Error(),
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
		}
	} else {
		// If the user is not an admin, log an error, and emit an error message back to the user.
		log.Warnf("Public user UUID %s not allowed to start bulk check task", ep.Kws.UUID)
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onBulkCheckPause(ep *socketio.EventPayload) {
	// onBulkCheckPause handles the bulk check pause event for websocket. It will be called when an admin user sends a bulk check pause message to the websocket.
	// It will log the event, and pause the bulk check task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s pause bulk check task", ep.Kws.UUID)
		// Pause the bulk check task.
		scheduler.PauseBulkCheckTask()
	} else {
		log.Warnf("Public user UUID %s not allowed to pause bulk check task", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onBulkCheckResume(ep *socketio.EventPayload) {
	// onBulkCheckResume handles the bulk check resume event for websocket. It will be called when an admin user sends a bulk check resume message to the websocket.
	// It will log the event, and resume the bulk check task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.
	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s resume bulk check task", ep.Kws.UUID)
		scheduler.ResumeBulkCheckTask()
	} else {
		log.Warnf("Public user UUID %s not allowed to resume bulk check task", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onBulkCheckCancel(ep *socketio.EventPayload) {
	// onBulkCheckCancel handles the bulk check cancel event for websocket. It will be called when an admin user sends a bulk check cancel message to the websocket.
	// It will log the event, and cancel the bulk check task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s cancel bulk check task", ep.Kws.UUID)
		// Cancel the bulk check task.
		scheduler.CancelBulkCheckTask()
	} else {
		log.Warnf("Public user UUID %s not allowed to cancel bulk check task", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onBulkCheckClear(ep *socketio.EventPayload) {
	// onBulkCheckClear handles the bulk check clear event for websocket. It will be called when an admin user sends a bulk check clear message to the websocket.
	// It will log the event, and clear the bulk check task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s clear bulk check task", ep.Kws.UUID)
		scheduler.ClearBulkCheckTask()
	} else {
		log.Warnf("Public user UUID %s not allowed to clear bulk check task", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onBulkRecheckErrorDomains(ep *socketio.EventPayload) {
	// onBulkRecheckErrorDomains handles the bulk recheck error domains event for websocket. It will be called when an admin user sends a bulk recheck error domains message to the websocket.
	// It will log the event, and requery the error domains of the bulk check task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s requery bulk check task error domains", ep.Kws.UUID)
		scheduler.RecheckBulkCheckErrorDomains()
	} else {
		log.Warnf("Public user UUID %s not allowed to requery bulk check task error domains", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "拒绝访问批量任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onWebCheck(ep *socketio.EventPayload) {
	// onWebCheck handles the web check message from websocket. It will be called when a user sends a web check message to the websocket.
	// It will log the event, unmarshal the message into a WebCheck, and run the web task if the user is in the client map.
	// If the user is not in the client map, it will log an error, and emit an error message back to the user.

	log.Debugf("Web check message from user %s is: %s", ep.Kws.UUID, string(ep.Data))

	checkMessage := WebCheck{}
	err := sonic.Unmarshal(ep.Data, &checkMessage)
	if err != nil {
		log.Warnf("Error unmarshalling websocket web check message from user %s: %s", ep.Kws.UUID, err)
		return
	}

	uniqueDomains := make([]string, 0)

	// Trim and get the main domain of the raw domains
	for _, domain := range checkMessage.Domains {
		if domain != "" {
			mainDomain, err := utils.TrimAndGetMainDomain(domain)
			if err == nil {
				if (mainDomain != "") && (!slice.Contain(uniqueDomains, mainDomain)) {
					uniqueDomains = append(uniqueDomains, mainDomain)
				}
			} else {
				log.Error("Skip invalid domain name: ", domain)
			}
		}
	}

	// Send the result to the user through the websocket
	wsMessage := map[string]interface{}{
		"event": constant.WebsocketResponseEventWebCheckDomains,
		"data":  uniqueDomains,
	}
	ep.Kws.Emit([]byte(convertor.ToString(wsMessage)), socketio.TextMessage)

	// Check if the user is in the client map.
	if clientInfo, ok := clients[ep.Kws.UUID]; ok {
		// If the user is in the client map and the web task does not exist, create a new web task.
		if clientInfo.WebCheckTask == nil {
			log.Infof("Create new web check task for user %s", ep.Kws.UUID)
			clientInfo.WebCheckTask = scheduler.NewWebCheckTask(ep.Kws.UUID, ep.Kws)
			clients[ep.Kws.UUID] = clientInfo
		}

		// Set the domains of the web task.
		clientInfo.WebCheckTask.SetDomains(uniqueDomains)

		// Run the web check task.
		go clientInfo.WebCheckTask.Run(checkMessage.QueryType)

		log.Debugf("Start web check task for user %s", ep.Kws.UUID)
	} else {
		log.Warnf("No client found for user %s", ep.Kws.UUID)
		// If the user is not in the client map, emit an error message back to the user.
		responseError := fiber.Map{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "服务端错误, 未找到客户端信息",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onTypoCheck(ep *socketio.EventPayload) {
	// onTypoCheck handles the typo check message from websocket. It will be called when a user sends a typo check message to the websocket.
	// It will log the event, unmarshal the message into a TypoCheck, and run the typo check task.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s request typo check", ep.Kws.UUID)
		typoMessage := TypoCheck{}
		err := sonic.Unmarshal(ep.Data, &typoMessage)
		if err != nil {
			log.Warnf("Error unmarshalling websocket typo check message from user %s: %s", ep.Kws.UUID, err)
			responseError := fiber.Map{
				"event": constant.WebsocketResponseTypoCheckErrorEvent,
				"data":  "服务端错误, 格式化请求数据失败",
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
			return
		}

		mainDomain, err := utils.TrimAndGetMainDomain(typoMessage.Domain)
		if err != nil {
			log.Errorf("Error trimming and getting main domain for typo check domain %s: %s", typoMessage.Domain, err)
			responseError := fiber.Map{
				"event": constant.WebsocketResponseTypoCheckErrorEvent,
				"data":  "域名格式错误，请重新输入",
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
			return
		}

		typoHandler := typo.Typo{
			Domain: mainDomain,
		}

		allTypoDomains := make([]string, 0)

		for _, typoType := range typoMessage.TypoType {
			typoDomains := make([]string, 0)

			switch typoType {
			case constant.TypoTypeWww:
				typoDomains = typoHandler.TypeWww()
			case constant.TypoTypeSkipLetter:
				typoDomains = typoHandler.TypeSkipLetter()
			case constant.TypoTypeDoubleLetter:
				typoDomains = typoHandler.TypeDoubleLetter()
			case constant.TypoTypeReverseLetter:
				typoDomains = typoHandler.TypeReverseLetter()
			case constant.TypoTypeInsertedLetter:
				typoDomains = typoHandler.TypeInsertedLetter()
			case constant.TypoTypeWrongHorizontalKey:
				typoDomains = typoHandler.TypeWrongHorizontalKey()
			case constant.TypoTypeWrongVerticalKey:
				typoDomains = typoHandler.TypeWrongVerticalKey()
			case constant.TypoTypeCustomizedReplace:
				typoDomains = typoHandler.TypeCustomizedReplace()
			}

			if len(typoDomains) > 0 {
				allTypoDomains = append(allTypoDomains, typoDomains...)
				responseData := fiber.Map{
					"event": constant.WebsocketResponseEventTypoResult,
					"data": fiber.Map{
						"typoType": typoType,
						"domains":  typoDomains,
					},
				}
				ep.Kws.Emit([]byte(convertor.ToString(responseData)), socketio.TextMessage)
			}
		}

		if len(typoMessage.CcTlds) > 0 {
			typoDomains := typoHandler.TypeWrongTlds(typoMessage.CcTlds)
			if len(typoDomains) > 0 {
				allTypoDomains = append(allTypoDomains, typoDomains...)
				responseData := fiber.Map{
					"event": constant.WebsocketResponseEventTypoResult,
					"data": fiber.Map{
						"typoType": constant.TypoTypeWrongTlds,
						"domains":  typoDomains,
					},
				}
				ep.Kws.Emit([]byte(convertor.ToString(responseData)), socketio.TextMessage)
			}
		}

		if len(allTypoDomains) > 0 {
			// Check if the user is in the client map.
			if clientInfo, ok := clients[ep.Kws.UUID]; ok {
				// If the user is in the client map and the web task does not exist, create a new web task.
				if clientInfo.WebCheckTask == nil {
					log.Infof("Create new typo web check task for user %s", ep.Kws.UUID)
					clientInfo.WebCheckTask = scheduler.NewWebCheckTask(ep.Kws.UUID, ep.Kws)
					clients[ep.Kws.UUID] = clientInfo
				}

				// Set the domains of the web task.
				uniqueDomains := slice.Unique(allTypoDomains)
				clientInfo.WebCheckTask.SetDomains(uniqueDomains)

				// Run the web check task.
				go clientInfo.WebCheckTask.Run(typoMessage.QueryType)

				log.Debugf("Start typo web check task for user %s", ep.Kws.UUID)
			} else {
				log.Warnf("No client found for user %s", ep.Kws.UUID)
				// If the user is not in the client map, emit an error message back to the user.
				responseError := fiber.Map{
					"event": constant.WebsocketResponseTypoCheckErrorEvent,
					"data":  "服务端错误, 未找到客户端信息",
				}
				ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
			}
		}
	} else {
		log.Warnf("Public user UUID %s not allowed to request typo check", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseTypoCheckErrorEvent,
			"data":  "拒绝访问拼写检查任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func onRegister(ep *socketio.EventPayload) {
	// onRegister handles the register message from websocket. It will be called when a user sends a register message to the websocket.
	// It will log the event, unmarshal the message into a Register, and run the register task.
	// If the user is not an admin, it will log an error, and emit an error message back to the user.

	isAdmin := ep.Kws.GetAttribute("isAdmin")
	if isAdmin.(bool) {
		log.Infof("Admin user UUID %s request register domains", ep.Kws.UUID)
		registerMessage := Register{}
		err := sonic.Unmarshal(ep.Data, &registerMessage)
		if err != nil {
			log.Warnf("Error unmarshalling websocket register message from user %s: %s", ep.Kws.UUID, err)
			responseError := fiber.Map{
				"event": constant.WebsocketResponseRegisterErrorEvent,
				"data":  "服务端错误, 格式化请求数据失败",
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
			return
		}

		// Check if the user is in the client map.
		if clientInfo, ok := clients[ep.Kws.UUID]; ok {
			// If the user is in the client map and the web task does not exist, create a new web task.
			if clientInfo.RegisterTask == nil {
				log.Infof("Create new register task for user %s", ep.Kws.UUID)
				clientInfo.RegisterTask = scheduler.NewRegisterTask(ep.Kws.UUID, ep.Kws)
				clients[ep.Kws.UUID] = clientInfo
			}

			// Set the domains of the register task and run it.
			clientInfo.RegisterTask.SetDomains(registerMessage.Domains)
			go clientInfo.RegisterTask.Run(registerMessage.RegisterType)
			log.Debugf("Start register task for user %s", ep.Kws.UUID)
		} else {
			log.Warnf("No client found for user %s", ep.Kws.UUID)
			// If the user is not in the client map, emit an error message back to the user.
			responseError := fiber.Map{
				"event": constant.WebsocketResponseRegisterErrorEvent,
				"data":  "服务端错误, 未找到客户端信息",
			}
			ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
		}
	} else {
		log.Warnf("Public user UUID %s not allowed to request register domains", ep.Kws.UUID)
		// If the user is not an admin, emit an error message back to the user.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseRegisterErrorEvent,
			"data":  "拒绝访问注册任务, 请先登录",
		}
		ep.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
	}
}

func WebSocketUpgrade(c *fiber.Ctx) error {
	// Check if the request is a WebSocket upgrade request.
	// If it is, set the "allowed" local to true and call the next handler in the chain.
	// If it is not, return ErrUpgradeRequired.

	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

func WebSocketHandler(kws *socketio.Websocket) {
	// Set the "token" attribute to the query parameter "token"
	token := kws.Query("token")
	kws.SetAttribute("token", token)

	// Create a new client info struct and add it to the clients map
	clients[kws.UUID] = clientInfo{Kws: kws}

	// If the token is empty, set the "isAdmin" attribute to false
	if token == "" {
		kws.SetAttribute("isAdmin", false)
	} else {
		// If the token is not empty, validate it and set the "isAdmin" attribute to true if it is valid
		isValid, err := ValidateToken(token)
		if err == nil && isValid {
			kws.SetAttribute("isAdmin", true)
			// Add the WebSocket to the bulk check task
			scheduler.BulkCheckAddKws(kws)
		} else {
			kws.SetAttribute("isAdmin", false)
		}
	}
}
