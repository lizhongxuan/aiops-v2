package appui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type HostAgentTokenStore interface {
	StoreHostAgentToken(ctx context.Context, hostID, token string) (string, error)
	ResolveHostAgentToken(ctx context.Context, ref string) (string, error)
}

type localHostAgentTokenStore struct {
	secretDir string
}

func NewLocalHostAgentTokenStore(secretDir string) HostAgentTokenStore {
	return localHostAgentTokenStore{secretDir: strings.TrimSpace(secretDir)}
}

func (s localHostAgentTokenStore) StoreHostAgentToken(ctx context.Context, hostID, token string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return "", fmt.Errorf("host-agent token is required")
	}
	if strings.TrimSpace(s.secretDir) == "" {
		return "", fmt.Errorf("secret directory is not configured")
	}
	ref := hostAgentTokenSecretRef(hostID)
	rel, err := secretRefPath(ref)
	if err != nil {
		return "", err
	}
	target := filepath.Join(s.secretDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("create host-agent token secret directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".host-agent-token-*")
	if err != nil {
		return "", fmt.Errorf("create host-agent token secret: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return "", fmt.Errorf("chmod host-agent token secret: %w", err)
	}
	if _, err := tmp.WriteString(trimmedToken); err != nil {
		cleanup()
		return "", fmt.Errorf("write host-agent token secret: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close host-agent token secret: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("store host-agent token secret: %w", err)
	}
	return ref, nil
}

func (s localHostAgentTokenStore) ResolveHostAgentToken(ctx context.Context, ref string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	rel, err := secretRefPath(ref)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.secretDir) == "" {
		return "", fmt.Errorf("secret directory is not configured")
	}
	source := filepath.Join(s.secretDir, rel)
	cleanSecretDir, err := filepath.Abs(s.secretDir)
	if err != nil {
		return "", err
	}
	cleanSource, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(cleanSource, cleanSecretDir+string(os.PathSeparator)) && cleanSource != cleanSecretDir {
		return "", fmt.Errorf("secret ref escapes secret directory")
	}
	data, err := os.ReadFile(cleanSource)
	if err != nil {
		return "", fmt.Errorf("read host-agent token secret %s: %w", ref, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("host-agent token secret %s is empty", ref)
	}
	return token, nil
}

func hostAgentTokenSecretRef(hostID string) string {
	trimmed := strings.TrimSpace(hostID)
	segment := safeSecretPathSegment(trimmed)
	if segment == "" {
		segment = "host"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "secret://" + path.Join("hosts", segment+"-"+hex.EncodeToString(sum[:6]), "host-agent-token")
}
