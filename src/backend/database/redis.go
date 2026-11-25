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

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// GetRedis returns a Redis client using environment configuration.
func GetRedis() (*redis.Client, error) {
	return GetRedisContext(context.Background())
}

// GetRedisContext connects to Redis while respecting context cancellation on startup.
func GetRedisContext(ctx context.Context) (*redis.Client, error) {
	host := envOrDefault("REDIS_HOST", "localhost")
	port := envOrDefault("REDIS_PORT", "6379")
	dbStr := envOrDefault("REDIS_DB", "0")

	db, err := strconv.Atoi(dbStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse REDIS_DB to an int: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     net.JoinHostPort(host, port),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})

	for retryCount := 1; retryCount <= maxConnectRetries; retryCount++ {
		_, err := rdb.Ping(ctx).Result()
		if err == nil {
			log.Infof("Connected to Redis %s:%s DB %d", host, port, db)
			return rdb, nil
		}
		log.Errorf("Failed to connect to Redis %s:%s DB %d, error: %s", host, port, db, err)
		if retryCount == maxConnectRetries || ctx.Err() != nil {
			break
		}
		log.Infof("Performing the %d retry after %.0f seconds", retryCount, retryInterval.Seconds())
		select {
		case <-ctx.Done():
			_ = rdb.Close()
			return nil, ctx.Err()
		case <-time.After(retryInterval):
		}
	}

	_ = rdb.Close()
	log.Errorf("Failed to connect to Redis %s:%s DB %d after %d retries", host, port, db, maxConnectRetries)
	return nil, errors.New("failed to connect to Redis")
}
