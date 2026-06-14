package hostops

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentMessageStore interface {
	Append(ctx context.Context, msg AgentMessage) (AgentMessage, error)
	ListByMission(ctx context.Context, missionID string) ([]AgentMessage, error)
	Replay(ctx context.Context, missionID string) ([]AgentMessage, error)
}

type InMemoryAgentMessageStore struct {
	mu       sync.RWMutex
	nextID   int64
	messages []AgentMessage
}

func NewInMemoryAgentMessageStore() *InMemoryAgentMessageStore {
	return &InMemoryAgentMessageStore{}
}

func (s *InMemoryAgentMessageStore) Append(_ context.Context, msg AgentMessage) (AgentMessage, error) {
	if s == nil {
		return AgentMessage{}, ErrInvalidAgentMessage
	}
	msg.MissionID = strings.TrimSpace(msg.MissionID)
	msg.FromAgentID = strings.TrimSpace(msg.FromAgentID)
	msg.ToAgentID = strings.TrimSpace(msg.ToAgentID)
	msg.CorrelationID = strings.TrimSpace(msg.CorrelationID)
	if msg.MissionID == "" || msg.FromAgentID == "" || msg.ToAgentID == "" || msg.Type == "" {
		return AgentMessage{}, ErrInvalidAgentMessage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	if msg.ID == "" {
		msg.ID = "agent-message-" + strconv.FormatInt(s.nextID, 10)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	if msg.PayloadDigest == "" {
		msg.PayloadDigest = agentPayloadDigest(msg.Payload)
	}
	s.messages = append(s.messages, cloneAgentMessage(msg))
	return cloneAgentMessage(msg), nil
}

func (s *InMemoryAgentMessageStore) ListByMission(_ context.Context, missionID string) ([]AgentMessage, error) {
	if s == nil {
		return nil, ErrInvalidAgentMessage
	}
	missionID = strings.TrimSpace(missionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []AgentMessage
	for _, msg := range s.messages {
		if msg.MissionID == missionID {
			out = append(out, cloneAgentMessage(msg))
		}
	}
	return out, nil
}

func (s *InMemoryAgentMessageStore) Replay(ctx context.Context, missionID string) ([]AgentMessage, error) {
	return s.ListByMission(ctx, missionID)
}

func agentPayloadDigest(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return digestText("")
	}
	return digestText(string(data))
}
