// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisBridgeSessionStore struct {
	client    *redis.Client
	namespace string
}

func newRedisBridgeSessionStore(ctx context.Context, cfg BridgeStatefulSessionsRedisConfig) (*redisBridgeSessionStore, error) {
	cfg = bridgeStatefulSessionRedisConfig(cfg)
	username, err := envValueIfConfigured(cfg.UsernameEnv, "username_env")
	if err != nil {
		return nil, err
	}
	password, err := envValueIfConfigured(cfg.PasswordEnv, "password_env")
	if err != nil {
		return nil, err
	}
	options := &redis.Options{
		Addr:         strings.TrimSpace(cfg.Address),
		Username:     username,
		Password:     password,
		DB:           cfg.DB,
		DialTimeout:  redisTimeout(cfg.ConnectTimeoutMS),
		ReadTimeout:  redisTimeout(cfg.ReadTimeoutMS),
		WriteTimeout: redisTimeout(cfg.WriteTimeoutMS),
	}
	if cfg.PoolSize > 0 {
		options.PoolSize = cfg.PoolSize
	}
	if cfg.TLS.Enabled {
		options.TLSConfig = &tls.Config{
			ServerName:         strings.TrimSpace(cfg.TLS.ServerName),
			InsecureSkipVerify: cfg.TLS.InsecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		}
	}
	client := redis.NewClient(options)
	pingCtx := ctx
	cancel := func() {}
	if cfg.ConnectTimeoutMS > 0 {
		pingCtx, cancel = context.WithTimeout(ctx, redisTimeout(cfg.ConnectTimeoutMS))
	}
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis session backend: %w", err)
	}
	return &redisBridgeSessionStore{client: client, namespace: strings.TrimSpace(cfg.Namespace)}, nil
}

func (s *redisBridgeSessionStore) Get(ctx context.Context, key string) (bridgeSessionEntry, bool, error) {
	if s == nil || s.client == nil || strings.TrimSpace(key) == "" {
		return bridgeSessionEntry{}, false, nil
	}
	value, err := s.client.Get(ctx, s.key(key)).Result()
	if err == redis.Nil {
		return bridgeSessionEntry{}, false, nil
	}
	if err != nil {
		return bridgeSessionEntry{}, false, err
	}
	if strings.TrimSpace(value) == "" {
		return bridgeSessionEntry{}, false, nil
	}
	return bridgeSessionEntry{PreviousResponseID: value}, true, nil
}

func (s *redisBridgeSessionStore) Set(ctx context.Context, key, previousResponseID string, ttl time.Duration, maxEntries int) error {
	if s == nil || s.client == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(previousResponseID) == "" {
		return nil
	}
	return s.client.Set(ctx, s.key(key), previousResponseID, ttl).Err()
}

func (s *redisBridgeSessionStore) Delete(ctx context.Context, key string) error {
	if s == nil || s.client == nil || strings.TrimSpace(key) == "" {
		return nil
	}
	return s.client.Del(ctx, s.key(key)).Err()
}

func (s *redisBridgeSessionStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *redisBridgeSessionStore) key(key string) string {
	return bridgeRedisSessionKey(s.namespace, key)
}

func bridgeRedisBackendKey(cfg BridgeStatefulSessionsRedisConfig) string {
	cfg = bridgeStatefulSessionRedisConfig(cfg)
	parts := []string{
		strings.TrimSpace(cfg.Address),
		strings.TrimSpace(cfg.Namespace),
		fmt.Sprintf("%d", cfg.DB),
		strings.TrimSpace(cfg.UsernameEnv),
		strings.TrimSpace(cfg.PasswordEnv),
		fmt.Sprintf("%t", cfg.TLS.Enabled),
		strings.TrimSpace(cfg.TLS.ServerName),
		fmt.Sprintf("%t", cfg.TLS.InsecureSkipVerify),
		fmt.Sprintf("%d", cfg.ConnectTimeoutMS),
		fmt.Sprintf("%d", cfg.ReadTimeoutMS),
		fmt.Sprintf("%d", cfg.WriteTimeoutMS),
		fmt.Sprintf("%d", cfg.PoolSize),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func bridgeRedisSessionKey(namespace, hashedSessionKey string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = defaultBridgeStatefulSessionRedisNamespace
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(hashedSessionKey)))
	return "smart-llmrouter:bridge-session:" + namespace + ":" + hex.EncodeToString(sum[:])
}

func envValueIfConfigured(envName, field string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return "", nil
	}
	value := os.Getenv(envName)
	if value == "" {
		return "", fmt.Errorf("redis session backend %s %s is set but the environment variable is empty or unset", field, envName)
	}
	return value, nil
}

func redisTimeout(ms int) time.Duration {
	if ms <= 0 {
		return 500 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
