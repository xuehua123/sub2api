package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const lobeHubLaunchTicketKeyPrefix = "lobehub:launch_ticket:"

type lobeHubLaunchStateStore struct {
	rdb *redis.Client
}

func NewLobeHubLaunchStateStore(rdb *redis.Client) service.LobeHubLaunchStateStore {
	return &lobeHubLaunchStateStore{rdb: rdb}
}

func (s *lobeHubLaunchStateStore) CreateLaunchTicket(ctx context.Context, ticket *service.LobeHubLaunchTicket, ttl time.Duration) (string, error) {
	payload, err := json.Marshal(ticket)
	if err != nil {
		return "", fmt.Errorf("marshal lobehub launch ticket: %w", err)
	}

	ticketID := uuid.NewString()
	if err := s.rdb.Set(ctx, lobeHubLaunchTicketKey(ticketID), payload, ttl).Err(); err != nil {
		return "", err
	}

	return ticketID, nil
}

func (s *lobeHubLaunchStateStore) ConsumeLaunchTicket(ctx context.Context, ticketID string) (*service.LobeHubLaunchTicket, error) {
	payload, err := s.rdb.GetDel(ctx, lobeHubLaunchTicketKey(ticketID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrLobeHubLaunchTicketNotFound
		}
		return nil, err
	}

	var ticket service.LobeHubLaunchTicket
	if err := json.Unmarshal([]byte(payload), &ticket); err != nil {
		return nil, fmt.Errorf("unmarshal lobehub launch ticket: %w", err)
	}

	return &ticket, nil
}

func lobeHubLaunchTicketKey(ticketID string) string {
	return lobeHubLaunchTicketKeyPrefix + ticketID
}
