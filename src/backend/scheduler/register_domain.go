package scheduler

import (
	"context"
	"fmt"
	"sync"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/register"

	"github.com/duke-git/lancet/v2/convertor"
	"github.com/gofiber/contrib/socketio"
)

const (
	miniRegisterConcurrencyLimit = 1
)

type RegisterTask struct {
	UserID     string
	Kws        *socketio.Websocket
	Ctx        context.Context
	CancelFunc context.CancelFunc
	mu         sync.Mutex
	stopped    bool
	Domains    []string
}

// NewRegisterTask creates a new RegisterTask instance
func NewRegisterTask(userId string, kws *socketio.Websocket) *RegisterTask {
	return &RegisterTask{
		UserID: userId,
		Kws:    kws,
	}
}

// SetDomains sets the domains of the RegisterTask
func (r *RegisterTask) SetDomains(domains []string) {
	r.Domains = domains
}

// Run the register task for the given user and register type.
func (r *RegisterTask) Run(registerType string, maySubmit func() bool) {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	r.Ctx = ctx
	r.CancelFunc = cancelFunc
	r.mu.Unlock()
	defer cancelFunc()
	if len(r.Domains) == 0 {
		log.Error("Empty domains, do nothing")
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseRegisterErrorEvent,
			"data":  "未提供注册域名",
		}
		r.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
		return
	}

	log.Infof("Register task for user %s domain count: %d", r.UserID, len(r.Domains))

	// Get the register concurrency limit from the config
	cfg := config.GetConfig()

	var apiInfo config.RegisterApi
	for _, api := range cfg.RegisterApis {
		if api.ApiName == registerType {
			apiInfo = api
			break
		}
	}

	if apiInfo.ApiName == "" {
		log.Debugf("Invalid register type: %s, no register api found", registerType)
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseRegisterErrorEvent,
			"data":  fmt.Sprintf("未找到注册名称为%s的API", registerType),
		}
		r.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
		return
	}

	var concurrencyLimit int
	if len(r.Domains) > apiInfo.ConcurrencyLimit {
		if apiInfo.ConcurrencyLimit > 0 {
			concurrencyLimit = apiInfo.ConcurrencyLimit
		} else {
			concurrencyLimit = miniRegisterConcurrencyLimit
		}
	} else {
		concurrencyLimit = len(r.Domains)
	}

	// Create a new context and a cancel function

	// Create a channel and a wait group
	ch := make(chan string, concurrencyLimit)
	var wg sync.WaitGroup

	log.Infof("Going to create total %d register workers for user %s", concurrencyLimit, r.UserID)

	// Start the web query workers
	for i := 0; i < concurrencyLimit; i++ {
		wg.Add(1)
		go r.registerHandler(i, ch, &wg, registerType, maySubmit)
	}

	// Send the domains to the web query workers
	for _, domain := range r.Domains {
		select {
		case <-r.Ctx.Done():
			log.Infof("Force stop register task for user %s", r.UserID)
			close(ch)
			return
		case ch <- domain:
		}
	}

	log.Infof("All domains sent to user %s register workers, going to wait for workers to finish", r.UserID)

	// Close the channel and wait for the workers to finish
	close(ch)
	wg.Wait()

	log.Infof("Register task for user %s finished", r.UserID)

	// Reset the register task
}

func (r *RegisterTask) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.CancelFunc != nil {
		log.Infof("Going to stop register task for user %s", r.UserID)
		r.CancelFunc()
	}
}

func (r *RegisterTask) registerHandler(i int, ch chan string, wg *sync.WaitGroup, registerType string, maySubmit func() bool) {
	defer wg.Done()

	handerSeq := i + 1

	log.Debugf("Start register handler %d for user %s", handerSeq, r.UserID)

	for {
		select {
		case <-r.Ctx.Done():
			// If the context is canceled, stop the goroutine
			log.Infof("Force stop register task handler %d for user %s", handerSeq, r.UserID)
			return
		case domain, ok := <-ch:
			if !ok || r.Ctx.Err() != nil {
				// If the channel is closed, stop the goroutine
				log.Debugf("Register task handler %d for user %s finished", handerSeq, r.UserID)
				return
			}

			log.Debugf("Register task handler %d for user %s, register domain %s", handerSeq, r.UserID, domain)

			registerResult, err := register.RegisterForSession(r.Ctx, domain, registerType, maySubmit)
			if err != nil {
				log.Errorf("Register domain %s error: %v", domain, err)
			}

			response := map[string]interface{}{
				"event": constant.WebsocketResponseEventRegisterResult,
				"data":  registerResult,
			}

			r.Kws.Emit([]byte(convertor.ToString(response)), socketio.TextMessage)
		}
	}
}
