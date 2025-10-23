package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"quaily-journalist/internal/model"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func periodZKey(source, period string) string {
	return fmt.Sprintf("news:source:%s:period:%s", source, period)
}

func itemKey(source, id string) string {
	return fmt.Sprintf("news:item:%s:%s", source, id)
}

func publishedKey(channel, period string) string {
	return fmt.Sprintf("news:published:%s:%s", channel, period)
}

func skipKey(channel, id string) string {
	return fmt.Sprintf("news:skip:%s:%s", channel, id)
}

// AddNews stores/updates a news item and adds it to the current period sorted set with a score.
func (s *RedisStore) AddNews(ctx context.Context, source, period string, item model.NewsItem, score float64) error {
	// Store item data
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, itemKey(source, item.ID), b, 7*24*time.Hour).Err(); err != nil { // expire after a week
		return err
	}
	// Add to sorted set
	z := &redis.Z{Score: score, Member: item.ID}
	return s.rdb.ZAdd(ctx, periodZKey(source, period), *z).Err()
}

// TopNews retrieves the top N items by score for a period and source.
func (s *RedisStore) TopNews(ctx context.Context, source, period string, n int) ([]model.WithScore, error) {
	ids, err := s.rdb.ZRevRangeWithScores(ctx, periodZKey(source, period), 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]model.WithScore, 0, len(ids))
	for _, z := range ids {
		id := z.Member.(string)
		b, err := s.rdb.Get(ctx, itemKey(source, id)).Bytes()
		if err != nil {
			return nil, err
		}
		var it model.NewsItem
		if err := json.Unmarshal(b, &it); err != nil {
			return nil, err
		}
		out = append(out, model.WithScore{Item: it, Score: z.Score})
	}
	return out, nil
}

func (s *RedisStore) IsPublished(ctx context.Context, channel, period string) (bool, error) {
	res, err := s.rdb.Get(ctx, publishedKey(channel, period)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return res == "1", nil
}

func (s *RedisStore) MarkPublished(ctx context.Context, channel, period string) error {
	return s.rdb.Set(ctx, publishedKey(channel, period), "1", 30*24*time.Hour).Err()
}

// IsSkipped returns true if the item is marked as skipped for the channel.
func (s *RedisStore) IsSkipped(ctx context.Context, channel, id string) (bool, error) {
	_, err := s.rdb.Get(ctx, skipKey(channel, id)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkSkipped marks an item as skipped for the channel for the given duration.
func (s *RedisStore) MarkSkipped(ctx context.Context, channel, id string, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	return s.rdb.Set(ctx, skipKey(channel, id), "1", d).Err()
}
