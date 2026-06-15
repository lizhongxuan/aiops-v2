package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

type hostAgentGRPCAuthRepository interface {
	GetHost(id string) (*store.HostRecord, error)
}

type hostAgentGRPCAuthenticator struct {
	repo hostAgentGRPCAuthRepository
}

func (a hostAgentGRPCAuthenticator) AuthenticateHostAgentGRPC(ctx context.Context, hostID, token string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if a.repo == nil {
		return fmt.Errorf("host repository is not configured")
	}
	hostID = strings.TrimSpace(hostID)
	token = strings.TrimSpace(token)
	if hostID == "" || token == "" {
		return appui.ErrHostAgentUnauthorized
	}
	host, err := a.repo.GetHost(hostID)
	if err != nil {
		return err
	}
	if host == nil {
		return fmt.Errorf("host %q not found", hostID)
	}
	if err := verifyHostAgentGRPCToken(host.AgentTokenRef, token); err != nil {
		return err
	}
	return nil
}

func verifyHostAgentGRPCToken(ref, token string) error {
	ref = strings.TrimSpace(ref)
	token = strings.TrimSpace(token)
	if ref == "" || token == "" {
		return appui.ErrHostAgentUnauthorized
	}
	if !strings.HasPrefix(strings.ToLower(ref), "sha256:") {
		return fmt.Errorf("unsupported host-agent token ref")
	}
	want := strings.TrimPrefix(strings.ToLower(ref), "sha256:")
	sum := sha256.Sum256([]byte(token))
	got := fmt.Sprintf("%x", sum[:])
	if len(want) == len(got) && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1 {
		return nil
	}
	return appui.ErrHostAgentUnauthorized
}
