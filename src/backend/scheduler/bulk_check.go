package scheduler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"time"
	"typonamer/config"
	"typonamer/constant"
	"typonamer/database"
	"typonamer/log"
	"typonamer/lookup/lookuper"
	"typonamer/lookup/lookuperror"
	"typonamer/lookup/lookupinfo"
	"typonamer/utils"

	"github.com/bytedance/sonic"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gofiber/contrib/socketio"
	"github.com/redis/go-redis/v9"
)

const (
	redisPipelineMaxBulkCheckDomainCount int = 100
	miniBulkCheckConcurrencyLimit        int = 3
	bulkCheckInfoTimerInterval           int = 1 // 1 seconds
)

var bulkCheckVar BulkCheck = BulkCheck{}
var rdb *redis.Client

// BulkCheck is a struct that represents a bulk check task.
// A bulk check task is a set of domains that need to be checked.
type BulkCheck struct {
	// Kws is a list of websocket connections.
	// The websocket connections are used to send the query results to the clients.
	Kws []*socketio.Websocket

	// TaskInfoTimer is a timer that is used to send the task status to the clients at regular intervals.
	// The timer is started when admin user is connected and stopped when all admin user is disconnected.
	TaskInfoTimer *time.Ticker

	// Ctx is the context of the batch task.
	// The context is used to cancel the batch task when the context is done.
	Ctx context.Context

	// CancelFunc is a function that can be used to cancel the batch task.
	// The function is set when the batch task is created and can be used to cancel the batch task at any time.
	CancelFunc context.CancelFunc

	// mux is a read-write mutex that is used to protect the access to the batch task struct.
	// The mutex is used to ensure that only one goroutine can access the batch task struct at a time.
	mux sync.RWMutex
}

type BulkCheckDomain struct {
	Domain string
	Order  int
}

type BulkCheckStatusInfo struct {
	Status        string
	QueryType     string
	TotalDomains  int64
	RemainDomains int64
	TakenDomains  int64
	FreeDomains   int64
	ErrorDomains  int64
}

// init is the entry point of the batch task package.
// It is responsible for initializing the package by connecting to Redis and initializing the batch task status.
func init() {
	// Get a Redis client
	client, err := database.GetRedis()
	if err != nil {
		log.Error("Failed to connect to Redis: ", err)
		// If there is an error connecting to Redis, exit the program.
		os.Exit(1)
	}

	// Set the Redis client to the global variable.
	rdb = client

	// Initialize the Redis bulk check task status.
	err = redisInit()
	if err != nil {
		log.Error("Failed to init redis: ", err)
		// If there is an error initializing the Redis bulk check task status, exit the program.
		os.Exit(1)
	}

	// Create a timer that will be used to send the task status to the clients at regular intervals.
	// Set the timer to send the task status every second.
	bulkCheckVar.mux.Lock()
	bulkCheckVar.TaskInfoTimer = time.NewTicker(time.Duration(bulkCheckInfoTimerInterval) * time.Second)
	// Stop the timer initially.
	bulkCheckVar.TaskInfoTimer.Stop()
	bulkCheckVar.mux.Unlock()

	// Start a goroutine to send the task status to the clients.
	go bulkCheckStatusSender()

	// Start a goroutine to handle the startup of the bulk check task.
	go startUpHandler()
}

// BulkCheckAddKws adds a new websocket connection to the bulk check.
// It is used to send the query results to the clients.
func BulkCheckAddKws(kws *socketio.Websocket) {
	bulkCheckVar.mux.Lock()
	defer bulkCheckVar.mux.Unlock()
	if len(bulkCheckVar.Kws) == 0 {
		log.Info("New admin websocket connected, start bulk check info timer")
		bulkCheckVar.TaskInfoTimer.Reset(time.Duration(bulkCheckInfoTimerInterval) * time.Second)
	}
	if !slice.Contain(bulkCheckVar.Kws, kws) {
		log.Debug("Add kws to bulk check: ", kws.UUID)
		bulkCheckVar.Kws = append(bulkCheckVar.Kws, kws)
	}
}

// BulkCheckRemoveKws removes a websocket connection from the bulk check.
// It is used to send the query results to the clients.
// If there is no websocket connection left, it will stop the timer that sends the task status to the clients.
func BulkCheckRemoveKws(kws *socketio.Websocket) {
	bulkCheckVar.mux.Lock()
	defer bulkCheckVar.mux.Unlock()
	bulkCheckVar.Kws = slice.Without(bulkCheckVar.Kws, kws)
	if len(bulkCheckVar.Kws) == 0 {
		log.Info("No admin websocket connected, stop bulk check info timer")
		bulkCheckVar.TaskInfoTimer.Stop()
	}
}

// BulkCheckAddRawDomains adds the raw domains to redis.
// It is used to store the raw domains for the bulk check.
// It will clean the raw domains from redis first.
// It will return an error if it fails to add the raw domains to redis.
func BulkCheckAddRawDomains(content *bytes.Buffer) error {
	ctx := context.Background()

	// Clean the raw domains from redis.
	err := clearBulkCheckRawDomains()
	if err != nil {
		return err
	}

	log.Debugf("Clean raw domain from redis: %s", constant.BulkCheckRawDomainsRedisKey)

	// Add the raw domains to redis.
	err = rdb.Set(ctx, constant.BulkCheckRawDomainsRedisKey, content.Bytes(), 0).Err()
	if err != nil {
		log.Errorf("Failed to add raw domain to redis: %s", err)
		return err
	}

	log.Debug("Add raw domain to redis: ", constant.BulkCheckRawDomainsRedisKey)

	// Set the bulk check status to "init" to indicate that the bulk check is initializing.
	err = setBulkCheckStatus(constant.BulkCheckStatusInit)
	return err
}

// SetBulkCheckQueryType sets the query type for the bulk check.
// It will check if the query type is allowed, and if it is, it will set the
// query type to redis. If it fails to set the query type to redis, it will
// return an error.
// If the query type is not allowed, it will return an error.
func SetBulkCheckQueryType(queryType string) error {
	ctx := context.Background()

	// Set the query type to redis.
	err := rdb.Set(ctx, constant.BulkCheckQueryTypeRedisKey, queryType, 0).Err()
	if err != nil {
		return errors.New("failed to set query type")
	}

	log.Infof("Set bulk check query type to: %s", queryType)

	return nil
}

// GetBulkCheckQueryType gets the query type for the bulk check task
func GetBulkCheckQueryType() string {
	ctx := context.Background()
	queryType := rdb.Get(ctx, constant.BulkCheckQueryTypeRedisKey).Val()

	log.Debugf("Got bulk check query type: %s", queryType)

	return queryType
}

// CreateBulkCheck creates a bulk check task by cleaning the bulk check task data,
// unique raw domains and start the bulk check task.
func CreateBulkCheckTask() {
	// Clean the bulk check task data.
	err := clearBulkCheckData()
	if err != nil {
		log.Error("Failed to clean bulk check task data")
		return
	}

	log.Info("Start to unique raw domains")

	// Unique the raw domains.
	err = bulkCheckUniqueRawDomains()
	if err != nil {
		log.Error("Failed to unique raw domains")
		// If failed to unique raw domains, set the bulk check task status to error.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "域名去重失败, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return
	}

	log.Info("Unique raw domains completed")

	// Start the batch task.
	go runBulkCheckTask()
}

// PauseBulkCheck pauses the bulk check task by stopping the bulk check task
// and setting the bulk check task status to paused.
func PauseBulkCheckTask() {
	log.Info("Pause bulk check task")
	stopBulkCheck()
	setBulkCheckStatus(constant.BulkCheckStatusPaused)
}

// ResumeBulkCheck resumes the bulk check task by running the bulk check task.
// It will log the event and send the task status to the clients.
func ResumeBulkCheckTask() {
	log.Info("Resume bulk check task")

	// Run the bulk check task.
	go runBulkCheckTask()
}

// CancelBulkCheck cancels the bulk check task by stopping the bulk check task
// and setting the bulk check task status to canceled.
func CancelBulkCheckTask() {
	log.Info("Cancel bulk check task")
	stopBulkCheck()
	setBulkCheckStatus(constant.BulkCheckStatusCanceled)
}

// ClearBulkCheck clears the bulk check by clearing the redis database
// and setting the bulk check status to idle.
func ClearBulkCheckTask() {
	ctx := context.Background()

	// Clear the redis database.
	err := rdb.FlushDB(ctx).Err()
	if err != nil {
		// If there is an error clearing the redis database, log the error.
		log.Errorf("Failed to clear redis: %s", err)
		return
	}

	// Log the event.
	log.Info("Clear redis all data successfully")

	// Initialize the redis bulk check task status.
	err = redisInit()
	if err != nil {
		// If there is an error initializing the redis bulk check task status, log the error.
		log.Errorf("Failed to init redis: %s", err)
		return
	}

	// Log the event.
	log.Info("Init redis successfully")

	// Set the bulk check task status to idle.
	setBulkCheckStatus(constant.BulkCheckStatusIdle)
}

// RecheckBulkCheckErrorDomains requeries the error domains of the bulk check task.
// It will check if there are any error domains in the redis database, and if
// there are, it will requery the error domains and save the result to redis.
func RecheckBulkCheckErrorDomains() {
	log.Debug("Requery bulk check task error domains")

	ctx := context.Background()

	errorDomainsResult := GetBulkCheckErrorDomains()
	if len(errorDomainsResult) == 0 {
		log.Info("No error domain found")
		return
	}

	var uniqueErrorDomains []BulkCheckDomain

	errorDomains := utils.GetOrderedQueryResult(errorDomainsResult)
	for _, domain := range errorDomains {
		uniqueErrorDomains = append(uniqueErrorDomains, BulkCheckDomain{
			Domain: domain.Domain,
			Order:  domain.Order,
		})
	}

	err := clearBulkCheckUniqueDomains()
	if err != nil {
		log.Error("Failed to clean bulk check task unique domains")
		return
	}

	err = clearBulkCheckErrorResult()
	if err != nil {
		log.Error("Failed to clean bulk check task error result")
		return
	}

	uniqueErrorDomainCount := len(uniqueErrorDomains)

	log.Info("Unique bulk check task error domain count: ", uniqueErrorDomainCount)

	pipe := rdb.TxPipeline()
	n := 0
	endNumber := uniqueErrorDomainCount - 1
	for i, domainInfo := range uniqueErrorDomains {
		domainStr := convertor.ToString(domainInfo)
		err = pipe.HSet(ctx, constant.BulkCheckUniqueDomainsRedisKey, domainInfo.Domain, domainStr).Err()
		if err != nil {
			log.Errorf("Failed to add unique error domain %s to redis pipeline: %s", domainInfo.Domain, err)
			return
		}

		n++
		if (n > redisPipelineMaxBulkCheckDomainCount) || (i == endNumber) {
			_, err = pipe.Exec(ctx)
			if err != nil {
				log.Errorf("Error saving unique error domains to redis: %s", err)
				return
			}

			pipe = rdb.TxPipeline()
			n = 0
		}
	}

	log.Info("All unique bulk check task error domain saved to redis")

	go runBulkCheckTask()
}

// redisInit is a function to initialize the redis database for the bulk check task.
// It is called when the bulk check task is started.
// It sets the bulk check task status to idle if it does not exist in redis.
// It also creates the redis key for the unique domain counter if it does not exist.
func redisInit() error {
	ctx := context.Background()

	// Get the bulk check task status from redis
	taskStatus, err := rdb.Get(ctx, constant.BulkCheckStatusRedisKey).Result()
	if err != nil {
		// If the bulk check task status does not exist in redis, set it to idle
		if err == redis.Nil {
			err = setBulkCheckStatus(constant.BulkCheckStatusIdle)
			if err != nil {
				log.Error("Failed to set redis bulk check task init status")
				return err
			}
		} else {
			log.Info("Failed to get redis bulk check task status: ", err)
			return err
		}
	} else {
		log.Info("Get redis bulk check task status: ", taskStatus)
	}

	// Get the unique domain counter from redis
	_, err = rdb.Get(ctx, constant.BulkCheckUniqueDomainsCountRedisKey).Result()
	if err != nil {
		// If the unique domain counter does not exist in redis, create it
		if err == redis.Nil {
			log.Infof("Redis key %s does not exist, create it", constant.BulkCheckUniqueDomainsCountRedisKey)
			err = rdb.Set(ctx, constant.BulkCheckUniqueDomainsCountRedisKey, 0, 0).Err()
			if err != nil {
				log.Errorf("Failed to create redis key %s: %s", constant.BulkCheckUniqueDomainsCountRedisKey, err)
				return err
			}
		} else {
			log.Errorf("Failed to get value of key %s from redis: %v", constant.BulkCheckUniqueDomainsCountRedisKey, err)
			return err
		}
	}

	return nil
}

// startUpHandler is a goroutine that will be started when the program starts.
// It is responsible for checking the bulk check task status from redis and
// restarting the bulk check task if it was running when the program exited.
func startUpHandler() {
	taskStatus, err := getBulkCheckStatus()
	if err != nil {
		log.Error("Failed to get bulk check task status from Redis: ", err)
		return
	}
	// If the bulk check task status is running, restart the bulk check task
	if taskStatus == constant.BulkCheckStatusRunning {
		log.Info("Previous bulk check task running, restart it")
		go runBulkCheckTask()
	}
}

// bulkCheckStatusSender is a goroutine that will be started when the program starts.
// It is responsible for sending the bulk check task info to all the websocket connections.
func bulkCheckStatusSender() {
	for range bulkCheckVar.TaskInfoTimer.C {
		taskInfo, err := getBulkCheckInfo()
		if err == nil {
			response := map[string]interface{}{
				"event": constant.WebsocketResponseBulkCheckInfoEvent,
				"data":  taskInfo,
			}
			bulkCheckSendWsMessage([]byte(convertor.ToString(response)))
		} else {
			log.Warn("Failed to get bulk check task info: ", err)
		}
	}
}

// getBulkCheckInfo returns the bulk check task info.
// It will return an error if it fails to get the bulk check task info.
func getBulkCheckInfo() (BulkCheckStatusInfo, error) {
	ctx := context.Background()

	// Get the bulk check task status from redis
	taskStatus, err := getBulkCheckStatus()
	if err != nil {
		log.Errorf("Failed to get bulk check task status from Redis: %v", err)
		return BulkCheckStatusInfo{}, err
	}

	queryType := GetBulkCheckQueryType()

	// Get the total domains from redis
	totalDomains, err := rdb.Get(ctx, constant.BulkCheckUniqueDomainsCountRedisKey).Int64()
	if err == redis.Nil {
		log.Debugf("Redis key %s does not exist", constant.BulkCheckUniqueDomainsCountRedisKey)
	} else if err != nil {
		log.Errorf("Failed to get total domains from redis: %v", err)
		return BulkCheckStatusInfo{}, err
	}

	// Get the remain domains from redis
	remainDomains := rdb.HLen(ctx, constant.BulkCheckUniqueDomainsRedisKey).Val()

	// Get the taken domains from redis
	takenDomains := rdb.LLen(ctx, constant.BulkCheckTakenResultRedisKey).Val()

	// Get the free domains from redis
	freeDomains := rdb.LLen(ctx, constant.BulkCheckFreeResultRedisKey).Val()

	// Get the error domains from redis
	errorDomains := rdb.LLen(ctx, constant.BulkCheckErrorResultRedisKey).Val()

	// Return the bulk check task info
	return BulkCheckStatusInfo{
		Status:        taskStatus,
		QueryType:     queryType,
		TotalDomains:  totalDomains,
		RemainDomains: remainDomains,
		TakenDomains:  takenDomains,
		FreeDomains:   freeDomains,
		ErrorDomains:  errorDomains,
	}, nil
}

// getBulkCheckStatus gets the bulk check task status from redis.
// It will return an error if the task status is not found.
func getBulkCheckStatus() (string, error) {
	ctx := context.Background()
	taskStatus := rdb.Get(ctx, constant.BulkCheckStatusRedisKey).Val()
	if taskStatus == "" {
		return "", errors.New("bulk check task status not found")
	}

	return taskStatus, nil
}

// setBulkCheckStatus sets the bulk check task status to redis.
// It will return an error if it fails to set the bulk check task status to redis.
func setBulkCheckStatus(status string) error {
	ctx := context.Background()
	err := rdb.Set(ctx, constant.BulkCheckStatusRedisKey, status, 0).Err()
	if err != nil {
		log.Errorf("Failed to set redis bulk check task status to %s: %s", status, err)
	} else {
		log.Info("Set redis bulk check task status to: ", status)
	}
	return err
}

// bulkCheckSendWsMessage sends a message to all the websocket connections of the bulk check task.
// It is thread-safe.
func bulkCheckSendWsMessage(message []byte) {
	bulkCheckVar.mux.RLock()
	defer bulkCheckVar.mux.RUnlock()
	if len(bulkCheckVar.Kws) > 0 {
		// Send the message to all the websocket connections of the bulk check task
		for _, kws := range bulkCheckVar.Kws {
			kws.Emit(message, socketio.TextMessage)
		}
	}
}

// clearBulkCheckData clears the bulk check task data from redis.
// It is called when the bulk check task is started.
// It will return an error if it fails to clean the bulk check task data from redis.
func clearBulkCheckData() error {
	// Clear the unique domains from redis
	err := clearBulkCheckUniqueDomains()
	if err != nil {
		// If failed to clean the unique domain from redis,
		// set the bulk check task status to error and send the error message to the websocket connections.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "服务端出现错误, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return err
	}

	log.Debug("Clean unique domain from redis: ", constant.BulkCheckUniqueDomainsRedisKey)

	// Clean the unique domain counter from redis
	err = clearBulkCheckUniqueDomainsCount()
	if err != nil {
		// If failed to clean the unique domain counter from redis,
		// set the bulk check task status to error and send the error message to the websocket connections.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "服务端出现错误, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return err
	}

	log.Debug("Clean unique domain counter from redis: ", constant.BulkCheckUniqueDomainsCountRedisKey)

	// Clean the taken result from redis
	err = clearBulkCheckTakenResult()
	if err != nil {
		// If failed to clean the taken result from redis,
		// set the bulk check task status to error and send the error message to the websocket connections.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "服务端出现错误, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return err
	}

	log.Debug("Clean taken result from redis: ", constant.BulkCheckTakenResultRedisKey)

	// Clean the free result from redis
	err = clearBulkCheckFreeResult()
	if err != nil {
		// If failed to clean the free result from redis,
		// set the bulk check task status to error and send the error message to the websocket connections.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "服务端出现错误, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return err
	}

	log.Debug("Clean free result from redis: ", constant.BulkCheckFreeResultRedisKey)

	// Clean the error result from redis
	err = clearBulkCheckErrorResult()
	if err != nil {
		// If failed to clean the error result from redis,
		// set the bulk check task status to error and send the error message to the websocket connections.
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "服务端出现错误, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return err
	}

	log.Debug("Clean error result from redis: ", constant.BulkCheckErrorResultRedisKey)

	return nil
}

// clearBulkCheckRawDomains clears the raw domains from redis.
func clearBulkCheckRawDomains() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckRawDomainsRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean raw domain from redis: %s", err)
		return err
	}
	return nil
}

// clearBulkCheckUniqueDomains clears the unique domains from redis.
func clearBulkCheckUniqueDomains() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckUniqueDomainsRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean unique domain from redis: %s", err)
		return err
	}
	return nil
}

// clearBulkCheckUniqueDomainsCount clears the unique domain counter from redis.
func clearBulkCheckUniqueDomainsCount() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckUniqueDomainsCountRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean unique domain counter from redis: %s", err)
		return err
	}
	return nil
}

// clearBulkCheckTakenResult clears the taken result from redis.
func clearBulkCheckTakenResult() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckTakenResultRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean taken result from redis: %s", err)
		return err
	}
	return nil
}

// clearBulkCheckFreeResult clears the free result from redis.
func clearBulkCheckFreeResult() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckFreeResultRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean free result from redis: %s", err)
		return err
	}
	return nil
}

// clearBulkCheckErrorResult clears the error result from redis.
func clearBulkCheckErrorResult() error {
	ctx := context.Background()
	err := rdb.Del(ctx, constant.BulkCheckErrorResultRedisKey).Err()
	if err != nil {
		log.Errorf("Failed to clean error result from redis: %s", err)
		return err
	}
	return nil
}

// stopBulkCheck stops the bulk check task.
// It will call the cancel function of the context of the bulk check task.
func stopBulkCheck() {
	bulkCheckVar.mux.Lock()
	defer bulkCheckVar.mux.Unlock()

	if bulkCheckVar.CancelFunc != nil {
		log.Info("Going to stop bulk check task")
		bulkCheckVar.CancelFunc()
	}
}

// bulkCheckUniqueRawDomains cleans the raw domains from redis and unique the raw domains to redis.
// It also sends the result to the websocket connections.
// It will return an error if it fails to add the raw domains to redis.
func bulkCheckUniqueRawDomains() error {
	err := setBulkCheckStatus(constant.BulkCheckStatusUniquing)
	if err != nil {
		return err
	}

	ctx := context.Background()
	mainDomainList := make([]string, 0)

	rawDomainsData, err := rdb.Get(ctx, constant.BulkCheckRawDomainsRedisKey).Bytes()
	if err != nil {
		log.Errorf("Failed to get raw domain from redis: %s", err)
		return err
	}

	// Read the raw domains from redis and trim/get the main domain of the raw domains.
	// Add the main domain to the mainDomainList.
	bytesReader := bytes.NewReader(rawDomainsData)
	reader := bufio.NewReader(bytesReader)
	for {
		line, _, err := reader.ReadLine()
		l := string(line)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Debugf("Failed to read raw domain from reader: %s", err)
			continue
		}

		mainDomain, err := utils.TrimAndGetMainDomain(l)
		if err == nil {
			if mainDomain != "" {
				mainDomainList = append(mainDomainList, mainDomain)
			} else {
				log.Debugf("Skip invalid domain name: %s", l)
			}
		} else {
			log.Debugf("Skip invalid domain name: %s", l)
		}
	}

	log.Info("Raw bulk check domain count: ", len(mainDomainList))

	// Remove duplicates from the mainDomainList.
	uniqueMainDomains := slice.Unique(mainDomainList)
	uniqueMainDomainCount := len(uniqueMainDomains)

	log.Info("Unique bulk check domain count: ", uniqueMainDomainCount)

	err = rdb.Set(ctx, constant.BulkCheckUniqueDomainsCountRedisKey, uniqueMainDomainCount, 0).Err()
	if err != nil {
		log.Errorf("Failed to set unique domain counter to redis: %s", err)
		return err
	}

	pipe := rdb.TxPipeline()
	n := 0
	endNumber := uniqueMainDomainCount - 1
	for i, domain := range uniqueMainDomains {
		domainInfo := BulkCheckDomain{
			Domain: domain,
			Order:  i,
		}

		domainStr := convertor.ToString(domainInfo)
		err = pipe.HSet(ctx, constant.BulkCheckUniqueDomainsRedisKey, domain, domainStr).Err()
		if err != nil {
			log.Errorf("Failed to add unique domain %s to redis pipeline: %s", domain, err)
			return err
		}

		n++
		if (n > redisPipelineMaxBulkCheckDomainCount) || (i == endNumber) {
			_, err = pipe.Exec(ctx)
			if err != nil {
				log.Errorf("Error saving unique domains to redis: %s", err)
				return err
			}

			pipe = rdb.TxPipeline()
			n = 0
		}
	}

	log.Info("All unique bulk check domain saved to redis")

	return nil
}

// runBulkCheck runs the bulk check task.
// It will get the unique domains from redis, starts the bulk check workers and waits for all workers to finish.
// It will set the batch task status to running when it starts and to done when all workers finish.
func runBulkCheckTask() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Bulk check task panic: %v", r)
			setBulkCheckStatus(constant.BulkCheckStatusError)
		}
	}()

	ctx := context.Background()

	uniqueDomains := rdb.HGetAll(ctx, constant.BulkCheckUniqueDomainsRedisKey).Val()
	if len(uniqueDomains) == 0 {
		log.Error("Unique domains is empty, do nothing")
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "域名去重后没有找到有效的域名, 请检查域名文件",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return
	}

	log.Debugf("Unique domains count: %d", len(uniqueDomains))

	queryType := GetBulkCheckQueryType()
	if queryType == "" {
		log.Error("Bulk check query type is empty")
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "获取查询类型失败, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return
	}

	cfg := config.GetConfig()

	var concurrencyLimit int
	if len(uniqueDomains) > cfg.BulkCheckConcurrencyLimit {
		if cfg.BulkCheckConcurrencyLimit > 0 {
			concurrencyLimit = cfg.BulkCheckConcurrencyLimit
		} else {
			concurrencyLimit = miniBulkCheckConcurrencyLimit
		}
	} else {
		concurrencyLimit = len(uniqueDomains)
	}

	err := setBulkCheckStatus(constant.BulkCheckStatusRunning)
	if err != nil {
		log.Error("Failed to set bulk check running status: ", err)
		responseError := map[string]interface{}{
			"event": constant.WebsocketResponseBulkCheckErrorEvent,
			"data":  "创建任务失败, 请检查服务端日志",
		}
		setBulkCheckStatus(constant.BulkCheckStatusError)
		bulkCheckSendWsMessage([]byte(convertor.ToString(responseError)))
		return
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	bulkCheckVar.mux.Lock()
	bulkCheckVar.Ctx = ctx
	bulkCheckVar.CancelFunc = cancelFunc
	bulkCheckVar.mux.Unlock()

	defer func() {
		bulkCheckVar.mux.Lock()
		bulkCheckVar.CancelFunc = nil
		bulkCheckVar.Ctx = nil
		bulkCheckVar.mux.Unlock()
	}()

	ch := make(chan BulkCheckDomain, concurrencyLimit)
	var wg sync.WaitGroup

	log.Infof("Going to create total %d bulk check workers", concurrencyLimit)

	for i := 0; i < concurrencyLimit; i++ {
		wg.Add(1)
		go bulkCheckQueryHandler(i, ch, &wg, queryType)
	}

	for _, domainStr := range uniqueDomains {
		select {
		case <-bulkCheckVar.Ctx.Done():
			log.Infof("Force stop bulk check task")
			close(ch)
			return
		default:
			domainItem := BulkCheckDomain{}
			err = sonic.Unmarshal([]byte(domainStr), &domainItem)
			if err != nil {
				log.Errorf("Failed to unmarshal redis unique domain JSON data '%s' to BulkCheckDomain object", domainStr)
				continue
			}
			ch <- domainItem
		}
	}

	log.Info("All domains sent to bulk check handlers, waiting for all handlers to finish")

	close(ch)
	wg.Wait()

	setBulkCheckStatus(constant.BulkCheckStatusDone)

	log.Info("Bulk check finished")
}

// bulkCheckQueryHandler is a goroutine function that queries the domains in the given channel and sends the result to redis.
// It also handles the cancellation of the context and the finish of the goroutine.
func bulkCheckQueryHandler(i int, ch chan BulkCheckDomain, wg *sync.WaitGroup, queryType string) {
	defer wg.Done()
	handerSeq := i + 1
	log.Debugf("Start bulk check handler %d", handerSeq)

	for {
		select {
		case <-bulkCheckVar.Ctx.Done():
			log.Infof("Force stop bulk check handler %d", handerSeq)
			return
		default:
			domainInfo, ok := <-ch
			if !ok {
				log.Debugf("Bulk check handler %d finished", handerSeq)
				return
			}

			log.Debugf("Bulk check handler %d query domain %s", handerSeq, domainInfo.Domain)

			lookupResult, err := lookuper.Lookup(domainInfo.Domain, queryType)
			bulkCheckLookupResultHandler(handerSeq, domainInfo, lookupResult, err)

			// Delete the domain from the unique domain list
			deleteDomainFromUniqueDomainList(domainInfo.Domain)
		}
	}
}

func bulkCheckLookupResultHandler(handerSeq int, domainInfo BulkCheckDomain, lookupResult lookupinfo.DomainInfo, lookupErr error) error {
	switch lookupResult.LookupType {
	case constant.LookupTypeWhois, constant.LookupTypeRDAP:
		if lookupErr == nil {
			// If the whois query is successful, parse the result and save it to redis
			queryResult := lookupinfo.QueryResult{
				Order:           domainInfo.Order,
				Domain:          domainInfo.Domain,
				LookupType:      lookupResult.LookupType,
				ViaProxy:        lookupResult.ViaProxy,
				RegisterStatus:  constant.DomainRegisterStatusTaken,
				CreatedDate:     lookupResult.CreationDate,
				ExpiryDate:      lookupResult.ExpiryDate,
				NameServer:      slice.Map(lookupResult.NameServer, utils.LowerString),
				DnsLite:         utils.GetDnsLite(lookupResult.NameServer),
				RawDomainStatus: lookupResult.DomainStatus,
				DomainStatus:    utils.GetDomainHumanStatus(lookupResult.DomainStatus),
			}

			log.Debugf("Bulk check whois query of domain %s result: %+v", domainInfo.Domain, queryResult)

			err := rdb.RPush(context.Background(), constant.BulkCheckTakenResultRedisKey, convertor.ToString(queryResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the whois taken result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		} else if errors.Is(lookupErr, lookuperror.ErrorWhoisNotFound) {
			// If the whois query is not successful because the domain is not found, set the register status to Free
			freeResult := lookupinfo.QueryResult{
				Order:          domainInfo.Order,
				Domain:         domainInfo.Domain,
				LookupType:     lookupResult.LookupType,
				ViaProxy:       lookupResult.ViaProxy,
				RegisterStatus: constant.DomainRegisterStatusFree,
			}

			log.Debugf("Bulk check whois query of domain %s result is free", domainInfo.Domain)

			err := rdb.RPush(context.Background(), constant.BulkCheckFreeResultRedisKey, convertor.ToString(freeResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the whois free result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		} else {
			log.Debugf("Bulk check whois query of domain %s error: %s", domainInfo.Domain, lookupErr)
			errorResult := lookupinfo.QueryResult{
				Order:          domainInfo.Order,
				Domain:         domainInfo.Domain,
				LookupType:     lookupResult.LookupType,
				ViaProxy:       lookupResult.ViaProxy,
				RegisterStatus: constant.DomainRegisterStatusError,
				QueryError:     utils.GetDomainHumanError(lookupErr),
			}
			err := rdb.RPush(context.Background(), constant.BulkCheckErrorResultRedisKey, convertor.ToString(errorResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the whois error result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		}
	case constant.LookupTypeDNS:
		if lookupErr == nil {
			if len(lookupResult.NameServer) > 0 {
				takenResult := lookupinfo.QueryResult{
					Order:          domainInfo.Order,
					Domain:         domainInfo.Domain,
					LookupType:     lookupResult.LookupType,
					RegisterStatus: constant.DomainRegisterStatusTaken,
					NameServer:     slice.Map(lookupResult.NameServer, utils.LowerString),
					DnsLite:        utils.GetDnsLite(lookupResult.NameServer),
				}

				log.Debugf("DNS query of domain %s taken result: %+v", domainInfo.Domain, takenResult)

				err := rdb.RPush(context.Background(), constant.BulkCheckTakenResultRedisKey, convertor.ToString(takenResult)).Err()
				if err != nil {
					log.Warnf("Bulk check handler %d failed to save the DNS taken result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
				}
				return err
			} else {
				freeResult := lookupinfo.QueryResult{
					Order:          domainInfo.Order,
					Domain:         domainInfo.Domain,
					LookupType:     lookupResult.LookupType,
					RegisterStatus: constant.DomainRegisterStatusFree,
				}

				log.Debugf("DNS query of domain %s free result: %+v", domainInfo.Domain, freeResult)

				err := rdb.RPush(context.Background(), constant.BulkCheckFreeResultRedisKey, convertor.ToString(freeResult)).Err()
				if err != nil {
					log.Warnf("Bulk check handler %d failed to save the DNS free result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
				}
				return err
			}
		} else if errors.Is(lookupErr, lookuperror.ErrorNsNotFound) {
			freeResult := lookupinfo.QueryResult{
				Order:          domainInfo.Order,
				Domain:         domainInfo.Domain,
				LookupType:     lookupResult.LookupType,
				RegisterStatus: constant.DomainRegisterStatusFree,
			}

			log.Debugf("DNS query of domain %s free result: %+v", domainInfo.Domain, freeResult)

			err := rdb.RPush(context.Background(), constant.BulkCheckFreeResultRedisKey, convertor.ToString(freeResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the DNS free result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		} else {
			log.Errorf("Dns query of domain %s error: %s", domainInfo.Domain, lookupErr)
			errorResult := lookupinfo.QueryResult{
				Order:          domainInfo.Order,
				Domain:         domainInfo.Domain,
				LookupType:     lookupResult.LookupType,
				RegisterStatus: constant.DomainRegisterStatusError,
				QueryError:     utils.GetDomainHumanError(lookupErr),
			}

			err := rdb.RPush(context.Background(), constant.BulkCheckErrorResultRedisKey, convertor.ToString(errorResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the DNS error result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		}
	default:
		// Customize api whois result
		if lookupErr != nil {
			log.Errorf("Customize api whois query of domain %s error: %s", domainInfo.Domain, lookupErr)
			errorResult := lookupinfo.QueryResult{
				Order:          domainInfo.Order,
				Domain:         domainInfo.Domain,
				LookupType:     lookupResult.LookupType,
				RegisterStatus: constant.DomainRegisterStatusError,
				QueryError:     utils.GetDomainHumanError(lookupErr),
			}

			err := rdb.RPush(context.Background(), constant.BulkCheckErrorResultRedisKey, convertor.ToString(errorResult)).Err()
			if err != nil {
				log.Warnf("Bulk check handler %d failed to save the customize api whois error result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
			}
			return err
		} else {
			switch lookupResult.CustomizedResult {
			case constant.DomainRegisterStatusTaken:
				takenResult := lookupinfo.QueryResult{
					Order:          domainInfo.Order,
					Domain:         domainInfo.Domain,
					LookupType:     lookupResult.LookupType,
					RegisterStatus: constant.DomainRegisterStatusTaken,
				}

				log.Debugf("Customize api whois query of domain %s taken result: %+v", domainInfo.Domain, takenResult)

				err := rdb.RPush(context.Background(), constant.BulkCheckTakenResultRedisKey, convertor.ToString(takenResult)).Err()
				if err != nil {
					log.Warnf("Bulk check handler %d failed to save the customize api whois taken result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
				}
				return err
			case constant.DomainRegisterStatusFree:
				freeResult := lookupinfo.QueryResult{
					Order:          domainInfo.Order,
					Domain:         domainInfo.Domain,
					LookupType:     lookupResult.LookupType,
					RegisterStatus: constant.DomainRegisterStatusFree,
				}

				log.Debugf("Customize api whois query of domain %s free result: %+v", domainInfo.Domain, freeResult)

				err := rdb.RPush(context.Background(), constant.BulkCheckFreeResultRedisKey, convertor.ToString(freeResult)).Err()
				if err != nil {
					log.Warnf("Bulk check handler %d failed to save the customize api whois free result of domain %s to redis: %s", handerSeq, domainInfo.Domain, err)
				}
				return err
			}
		}

		return nil
	}
}

// deleteDomainFromUniqueDomainList deletes a domain from the unique domain list in redis.
// It returns an error if it fails to delete the domain from redis.
func deleteDomainFromUniqueDomainList(domain string) error {
	// Delete the domain from the unique domain list in redis.
	err := rdb.HDel(context.Background(), constant.BulkCheckUniqueDomainsRedisKey, domain).Err()
	if err != nil {
		// Log the error if it fails to delete the domain from redis.
		log.Errorf("Failed to delete unique domain %s from redis: %s", domain, err)
	} else {
		// Log the success if it succeeds to delete the domain from redis.
		log.Infof("Delete unique domain %s from redis success", domain)
	}
	return err
}

func GetBulkCheckTakenDomains() []string {
	takenDomains := rdb.LRange(context.Background(), constant.BulkCheckTakenResultRedisKey, 0, -1).Val()
	return takenDomains
}

func GetBulkCheckFreeDomains() []string {
	freeDomains := rdb.LRange(context.Background(), constant.BulkCheckFreeResultRedisKey, 0, -1).Val()
	return freeDomains
}

func GetBulkCheckErrorDomains() []string {
	errorDomains := rdb.LRange(context.Background(), constant.BulkCheckErrorResultRedisKey, 0, -1).Val()
	return errorDomains
}
