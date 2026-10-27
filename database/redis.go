package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ConnectionOptions struct {
	Address  string
	Password string
	Database int
}

const pingTimeout = 5 * time.Second

func Open(ctx context.Context, options ConnectionOptions) (*redis.Client, error) {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     options.Address,
		Password: options.Password,
		DB:       options.Database,
	})

	pingContext, cancelPing := context.WithTimeout(ctx, pingTimeout)
	defer cancelPing()
	if err := redisClient.Ping(pingContext).Err(); err != nil {
		redisClient.Close()
		return nil, fmt.Errorf("connect redis %s db %d: %w", options.Address, options.Database, err)
	}
	return redisClient, nil
}
