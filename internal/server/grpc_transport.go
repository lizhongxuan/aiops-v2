package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"aiops-v2/internal/agentrpc"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

func RegisterAgentGRPCService(grpcServer *grpc.Server, manager *GRPCServer) {
	if grpcServer == nil || manager == nil {
		return
	}
	agentrpc.RegisterAgentServiceServer(grpcServer, &agentRPCService{manager: manager})
}

type agentRPCService struct {
	manager *GRPCServer
}

func (s *agentRPCService) Connect(stream agentrpc.AgentServiceConnectServer) error {
	return s.manager.HandleStream(grpcStructStream{stream: stream})
}

type grpcStructStream struct {
	stream agentrpc.AgentServiceConnectServer
}

func (s grpcStructStream) Send(msg *HostMessage) error {
	envelope, err := hostMessageToStruct(msg)
	if err != nil {
		return err
	}
	return s.stream.Send(envelope)
}

func (s grpcStructStream) Recv() (*HostMessage, error) {
	envelope, err := s.stream.Recv()
	if err != nil {
		return nil, err
	}
	return structToHostMessage(envelope)
}

func (s grpcStructStream) Context() context.Context {
	return s.stream.Context()
}

func hostMessageToStruct(msg *HostMessage) (*structpb.Struct, error) {
	if msg == nil {
		return nil, fmt.Errorf("host message is nil")
	}
	values := map[string]any{
		"type": msg.Type,
		"id":   msg.ID,
		"time": float64(msg.Time),
	}
	if len(msg.Payload) > 0 {
		values["payload"] = string(msg.Payload)
	}
	if msg.Error != "" {
		values["error"] = msg.Error
	}
	return structpb.NewStruct(values)
}

func structToHostMessage(value *structpb.Struct) (*HostMessage, error) {
	if value == nil {
		return nil, fmt.Errorf("empty host message")
	}
	fields := value.AsMap()
	msg := &HostMessage{
		Type:  stringMapValue(fields, "type"),
		ID:    stringMapValue(fields, "id"),
		Error: stringMapValue(fields, "error"),
	}
	if raw := stringMapValue(fields, "payload"); raw != "" {
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("host message payload is not JSON")
		}
		msg.Payload = json.RawMessage(raw)
	}
	if timestamp, ok := fields["time"].(float64); ok && !math.IsNaN(timestamp) {
		msg.Time = int64(timestamp)
	}
	return msg, nil
}

func stringMapValue(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}
