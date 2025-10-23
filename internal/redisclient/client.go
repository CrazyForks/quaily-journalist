package redisclient

import (
	"quaily-journalist/internal/config"

	"github.com/redis/go-redis/v9"
)

// New creates a Redis client from configuration.
func New(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
