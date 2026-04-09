package app

import (
	"context"

	"github.com/redis/go-redis/v9"
)

func scanKeys(ctx context.Context, client *redis.Client, pattern string, visit func(string) error) error {
	var cursor uint64

	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		for _, key := range keys {
			if err := visit(key); err != nil {
				return err
			}
		}

		if nextCursor == 0 {
			return nil
		}
		cursor = nextCursor
	}
}
