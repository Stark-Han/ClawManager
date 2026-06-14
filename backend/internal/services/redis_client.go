package services

import (
	"context"
	"time"
)

type PlatformRedisClient interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Del(ctx context.Context, key string) error
	XAdd(ctx context.Context, key string, fields map[string]string) (string, error)
	XRead(ctx context.Context, key, lastID string, block time.Duration) ([]redisStreamMessage, error)
}

func NewPlatformRedisClient(rawURL string) (PlatformRedisClient, error) {
	return newRedisBus(rawURL)
}
