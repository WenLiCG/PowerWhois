package database

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

func GetRedis(ctx context.Context) (*redis.Client, error) {
	host, port, db := os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT"), os.Getenv("REDIS_DB")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "6379"
	}
	if db == "" {
		db = "0"
	}
	index, err := strconv.Atoi(db)
	if err != nil || index < 0 {
		return nil, fmt.Errorf("invalid REDIS_DB")
	}
	client := redis.NewClient(&redis.Options{
		Addr: net.JoinHostPort(host, port), DB: index,
		Username: os.Getenv("REDIS_USERNAME"), Password: os.Getenv("REDIS_PASSWORD"),
		ContextTimeoutEnabled: true,
	})
	for i := 0; i < 10; i++ {
		if err = client.Ping(ctx).Err(); err == nil {
			return client, nil
		}
		if i == 9 {
			break
		}
		select {
		case <-ctx.Done():
			client.Close()
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	client.Close()
	return nil, fmt.Errorf("could not connect to Redis: %w", err)
}
