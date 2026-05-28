package mapcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/redisclient"
)

var ErrMapNotFound = errors.New("level map not found")

type Store struct {
	client *redisclient.Client
	ttl    time.Duration
}

func New(redisURL string, ttl time.Duration) (*Store, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, nil
	}
	client, err := redisclient.New(redisURL)
	if err != nil {
		return nil, err
	}
	return &Store{
		client: client,
		ttl:    ttl,
	}, nil
}

func (s *Store) Get(ctx context.Context, version int, generationID string) (mapgen.GeneratedMap, error) {
	_ = ctx
	raw, err := s.client.Do("GET", key(version, generationID))
	if errors.Is(err, redisclient.ErrNil) {
		return mapgen.GeneratedMap{}, ErrMapNotFound
	}
	if err != nil {
		return mapgen.GeneratedMap{}, err
	}

	body, ok := raw.(string)
	if !ok {
		return mapgen.GeneratedMap{}, fmt.Errorf("unexpected redis map payload type %T", raw)
	}

	var levelMap mapgen.GeneratedMap
	if err := json.Unmarshal([]byte(body), &levelMap); err != nil {
		return mapgen.GeneratedMap{}, err
	}
	return levelMap, nil
}

func (s *Store) Set(ctx context.Context, generationID string, levelMap mapgen.GeneratedMap) error {
	_ = ctx
	body, err := json.Marshal(levelMap)
	if err != nil {
		return err
	}
	_, err = s.client.Do("SET", key(levelMap.Version, generationID), string(body), "EX", strconv.Itoa(int(s.ttl.Seconds())))
	return err
}

func (s *Store) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func key(version int, generationID string) string {
	return fmt.Sprintf("level-map:v%d:generation:%s", version, generationID)
}
