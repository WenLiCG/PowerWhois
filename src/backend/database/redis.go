package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"typonamer/log"

	"github.com/redis/go-redis/v9"
)

const (
	// maxConnectRetries is the maximum number of times to retry connecting to the Redis DB.
	maxConnectRetries = 10

	// retryInterval is the time to wait between retries when connecting to the Redis DB.
	retryInterval = 5 * time.Second
)

var (
	redisHost string = os.Getenv("REDIS_HOST")
	redisPort string = os.Getenv("REDIS_PORT")
	redisDB   string = os.Getenv("REDIS_DB")
)

func init() {
	// If the REDIS_HOST environment variable is not set, set it to the default
	// value which is "localhost".
	if redisHost == "" {
		redisHost = "localhost"
	}

	// If the REDIS_PORT environment variable is not set, set it to the default
	// value which is "6379".
	if redisPort == "" {
		redisPort = "6379"
	}

	// If the REDIS_DB environment variable is not set, set it to the default
	// value which is "0".
	if redisDB == "" {
		redisDB = "0"
	}
}

// GetRedis returns a Redis client. It will try to connect to the Redis DB
// specified by the REDIS_HOST, REDIS_PORT and REDIS_DB environment variables.
// If the connection to the Redis DB fails, it will retry connecting to the Redis DB
// maxConnectRetries times with a delay of retryInterval between retries.
func GetRedis() (*redis.Client, error) {
	// Parse the REDIS_DB environment variable to an int.
	db, err := strconv.Atoi(redisDB)
	if err != nil {
		return nil, fmt.Errorf("failed to parse REDIS_DB to an int: %w", err)
	}

	// Create a Redis client with the specified host and port.
	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(redisHost, redisPort),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})

	// Initialize the retry count.
	retryCount := 1

	// Try to connect to the Redis DB.
	for {
		// If the retry count is less than maxConnectRetries, try to ping the Redis DB.
		// If the ping is successful, return the Redis client.
		// If the ping fails, log the error and retry after retryInterval.
		if retryCount < maxConnectRetries {
			_, err := rdb.Ping(context.Background()).Result()
			if err == nil {
				log.Infof("Connected to Redis %s:%s DB %d", redisHost, redisPort, db)
				return rdb, nil
			} else {
				log.Errorf("Failed to connect to Redis %s:%s DB %d, error: %s", redisHost, redisPort, db, err)
				log.Infof("Performing the %d retry after %f seconds", retryCount, retryInterval.Seconds())
				time.Sleep(retryInterval)
			}
		} else {
			log.Errorf("Failed to connect to Redis %s:%s DB %d after %d retries", redisHost, redisPort, db, maxConnectRetries)
			return nil, errors.New("failed to connect to Redis")
		}
		retryCount++
	}
}
