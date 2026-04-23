package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const ttl = 5 * time.Minute

func OccupancyKey(hotelId int64, year, month int, venueType string) string {
	return fmt.Sprintf("dashboard:occupancy:%d:%d:%02d:%s", hotelId, year, month, venueType)
}

func ActivityKey(hotelId int64, year, month int, venueType string) string {
	return fmt.Sprintf("dashboard:activity:%d:%d:%02d:%s", hotelId, year, month, venueType)
}

func ThresholdKey(hotelId int64) string {
	return fmt.Sprintf("config:threshold:%d", hotelId)
}

func Get(ctx context.Context, rdb *redis.Client, key string, dest any) bool {
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	return json.Unmarshal([]byte(val), dest) == nil
}

func Set(ctx context.Context, rdb *redis.Client, key string, data any) {
	b, _ := json.Marshal(data)
	rdb.Set(ctx, key, b, ttl)
}

func DeletePattern(ctx context.Context, rdb *redis.Client, pattern string) {
	keys, err := rdb.Keys(ctx, pattern).Result()
	if err != nil || len(keys) == 0 {
		return
	}
	rdb.Del(ctx, keys...)
}
