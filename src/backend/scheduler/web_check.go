package scheduler

import (
	"context"
	"errors"
	"sync"

	"typonamer/config"
	"typonamer/constant"
	"typonamer/log"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gofiber/contrib/socketio"
)

const (
	miniWebCheckConcurrencyLimit = 1
)

type WebCheck struct {
	UserID     string
	Kws        *socketio.Websocket
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Domains    []string
}

// NewWebCheck creates a new WebCheck instance
func NewWebCheckTask(userId string, kws *socketio.Websocket) *WebCheck {
	return &WebCheck{
		UserID: userId,
		Kws:    kws,
	}
}

// SetDomains sets the domains of the WebCheck
func (t *WebCheck) SetDomains(domains []string) {
	t.Domains = domains
}

// Run runs the web check task for the given user and query type.
// It trims and gets the main domain of the raw domains, and sends the result to the user through the websocket.
// It also starts the web query workers to query the domains.
func (t *WebCheck) Run(queryType string) {
	if len(t.Domains) == 0 {
		log.Error("Empty domains, do nothing")
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseWebCheckErrorEvent,
			"data":  "未输入查询域名",
		}
		t.Kws.Emit([]byte(convertor.ToString(responseError)), socketio.TextMessage)
		return
	}

	log.Infof("Web check task for user %s domain count: %d", t.UserID, len(t.Domains))

	// Get the web query concurrency limit from the config
	cfg := config.GetConfig()

	var concurrencyLimit int
	if len(t.Domains) > cfg.WebCheckDomainLimit {
		if cfg.WebCheckDomainLimit > 0 {
			concurrencyLimit = cfg.WebCheckDomainLimit
		} else {
			concurrencyLimit = miniWebCheckConcurrencyLimit
		}
	} else {
		concurrencyLimit = len(t.Domains)
	}

	// Create a new context and a cancel function
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Ctx = ctx
	t.CancelFunc = cancelFunc

	// Create a channel and a wait group
	ch := make(chan string, concurrencyLimit)
	var wg sync.WaitGroup

	log.Infof("Going to create total %d web query workers for user %s", concurrencyLimit, t.UserID)

	// Start the web query workers
	for i := 0; i < concurrencyLimit; i++ {
		wg.Add(1)
		go t.webCheckHandler(i, ch, &wg, queryType)
	}

	// Send the domains to the web query workers
	for _, domain := range t.Domains {
		select {
		case <-t.Ctx.Done():
			log.Infof("Force stop web check task for user %s", t.UserID)
			close(ch)
			return
		default:
			ch <- domain
		}
	}

	log.Infof("All domains sent to user %s web query workers, going to wait for workers to finish", t.UserID)

	// Close the channel and wait for the workers to finish
	close(ch)
	wg.Wait()

	log.Infof("Web check task for user %s finished", t.UserID)

	// Reset the web check task
	// t.CancelFunc = nil
	// t.Ctx = nil
	t.Domains = []string{}
}

// Stop stops the web check task.
// It cancels the context and reset the web check task.
func (t *WebCheck) Stop() {
	if t.CancelFunc != nil {
		log.Infof("Going to stop web check task for user %s", t.UserID)
		t.CancelFunc()
	}
	t.Domains = []string{}
}

// webCheckHandler is a goroutine function that queries the domains in the given channel and sends the result to the user through the websocket.
// It also handles the cancellation of the context and the finish of the goroutine.
func (t *WebCheck) webCheckHandler(i int, ch chan string, wg *sync.WaitGroup, queryType string) {
	defer wg.Done()

	handerSeq := i + 1

	log.Debugf("Start web check handler %d for user %s", handerSeq, t.UserID)

	for {
		select {
		case <-t.Ctx.Done():
			// If the context is canceled, stop the goroutine
			log.Infof("Force stop web check task handler %d for user %s", handerSeq, t.UserID)
			return
		default:
			domain, ok := <-ch
			if !ok {
				// If the channel is closed, stop the goroutine
				log.Debugf("Web check task handler %d for user %s finished", handerSeq, t.UserID)
				return
			}

			log.Debugf("Web check task handler %d for user %s, query domain %s", handerSeq, t.UserID, domain)

			lookupResult, err := lookuper.Lookup(domain, queryType)
			t.webLookupResultHandler(domain, lookupResult, err)
		}
	}
}

func (t *WebCheck) webLookupResultHandler(domain string, lookupResult lookupinfo.DomainInfo, lookupErr error) {
	queryResult := lookupinfo.QueryResult{
		Domain:      domain,
		LookupType:  lookupResult.LookupType,
		ViaProxy:    lookupResult.ViaProxy,
		RawResponse: lookupResult.RawResponse,
	}

	switch lookupResult.LookupType {
	case constant.LookupTypeWhois, constant.LookupTypeRDAP:
		if lookupErr == nil {
			// If the whois query is successful, parse the result and send it to the user through the websocket
			queryResult.RegisterStatus = constant.DomainRegisterStatusTaken
			queryResult.CreatedDate = lookupResult.CreationDate
			queryResult.ExpiryDate = lookupResult.ExpiryDate
			queryResult.NameServer = slice.Map(lookupResult.NameServer, utils.LowerString)
			queryResult.DnsLite = utils.GetDnsLite(lookupResult.NameServer)
			queryResult.RawDomainStatus = lookupResult.DomainStatus
			queryResult.DomainStatus = utils.GetDomainHumanStatus(lookupResult.DomainStatus)
		} else if errors.Is(lookupErr, lookuperror.ErrorWhoisNotFound) {
			// If the whois query is not successful because the domain is not found, set the register status to Free
			queryResult.RegisterStatus = constant.DomainRegisterStatusFree
		} else {
			// If the whois query is not successful because of other errors, set the register status to Error and the query error to the error message
			queryResult.RegisterStatus = constant.DomainRegisterStatusError
			queryResult.QueryError = utils.GetDomainHumanError(lookupErr)
		}
	case constant.LookupTypeDNS:
		if lookupErr == nil {
			if len(lookupResult.NameServer) > 0 {
				queryResult.RegisterStatus = constant.DomainRegisterStatusTaken
				// Convert the name server list to lowercase and save it to the NameServer field
				queryResult.NameServer = slice.Map(lookupResult.NameServer, utils.LowerString)
				// Calculate the DNS lite of the name server list and save it to the DnsLite field
				queryResult.DnsLite = utils.GetDnsLite(lookupResult.NameServer)
			} else {
				queryResult.RegisterStatus = constant.DomainRegisterStatusFree
			}
		} else if errors.Is(lookupErr, lookuperror.ErrorNsNotFound) {
			queryResult.RegisterStatus = constant.DomainRegisterStatusFree
		} else {
			queryResult.RegisterStatus = constant.DomainRegisterStatusError
			// Convert the DNS error to human readable error message and save it to the QueryError field
			queryResult.QueryError = utils.GetDomainHumanError(lookupErr)
		}
	default:
		// Customize api whois result
		if lookupErr != nil {
			queryResult.RegisterStatus = constant.DomainRegisterStatusError
			queryResult.QueryError = utils.GetDomainHumanError(lookupErr)
		} else {
			queryResult.RegisterStatus = lookupResult.CustomizedResult
		}
	}

	log.Debugf("Web lookup of domain %s result: %+v", domain, queryResult)

	// Send the result to the user through the websocket
	response := map[string]interface{}{
		"event": constant.WebsocketResponseEventWebCheckResult,
		"data":  queryResult,
	}

	t.Kws.Emit([]byte(convertor.ToString(response)), socketio.TextMessage)
}
