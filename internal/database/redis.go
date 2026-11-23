package database

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

func NewRedisClient(redisURL string) (*RedisClient, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[WARN] Redis ping failed (Redis may still be starting): %v", err)
	} else {
		log.Println("[Redis] Successfully connected to Redis.")
	}

	return &RedisClient{Client: rdb}, nil
}

func (r *RedisClient) Close() error {
	return r.Client.Close()
}
