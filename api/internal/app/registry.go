package app

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

func (s *Server) startRegistryCleanup(ctx context.Context) {
	startCleanupTicker(ctx, s.cfg.RegistryCleanupInterval, s.cleanupStaleRegistryEntries, func(err error) {
		s.logger.Printf("registry cleanup failed: %v", err)
	})
}

func (s *Server) cleanupStaleRegistryEntries(ctx context.Context) error {
	records, err := s.loadReadyPods(ctx)
	if err != nil {
		return err
	}

	for ip := range records {
		heartbeatExists, err := s.redis.Exists(ctx, heartbeatKey(ip)).Result()
		if err != nil {
			return err
		}
		if heartbeatExists > 0 {
			continue
		}

		if err := s.redis.HDel(ctx, podRegistryKey, ip).Err(); err != nil && err != redis.Nil {
			return err
		}
		if err := s.deleteLobbiesForPod(ctx, ip); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) deleteLobbiesForPod(ctx context.Context, podIP string) error {
	return scanKeys(ctx, s.redis, "lobby:*", func(key string) error {
		values, err := s.redis.HGetAll(ctx, key).Result()
		if err != nil || len(values) == 0 {
			return nil
		}
		if strings.TrimSpace(values["pod_ip"]) != podIP {
			return nil
		}
		if err := s.redis.Del(ctx, key).Err(); err != nil {
			return err
		}
		return nil
	})
}

func heartbeatKey(ip string) string {
	return "pod:heartbeat:" + ip
}
