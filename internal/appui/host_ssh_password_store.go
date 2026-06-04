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

type HostSSHPasswordStore interface {
	StoreHostSSHPassword(ctx context.Context, hostID, password string) (string, error)
}

type localHostSSHPasswordStore struct {
	secretDir string
}

func NewLocalHostSSHPasswordStore(secretDir string) HostSSHPasswordStore {
	return localHostSSHPasswordStore{secretDir: strings.TrimSpace(secretDir)}
}

func (s localHostSSHPasswordStore) StoreHostSSHPassword(ctx context.Context, hostID, password string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	trimmedPassword := strings.TrimSpace(password)
	if trimmedPassword == "" {
		return "", fmt.Errorf("ssh password is required")
	}
	if strings.TrimSpace(s.secretDir) == "" {
		return "", fmt.Errorf("secret directory is not configured")
	}
	ref := hostSSHPasswordSecretRef(hostID)
	rel, err := secretRefPath(ref)
	if err != nil {
		return "", err
	}
	target := filepath.Join(s.secretDir, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("create ssh password secret directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".ssh-password-*")
	if err != nil {
		return "", fmt.Errorf("create ssh password secret: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return "", fmt.Errorf("chmod ssh password secret: %w", err)
	}
	if _, err := tmp.WriteString(trimmedPassword); err != nil {
		cleanup()
		return "", fmt.Errorf("write ssh password secret: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close ssh password secret: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("store ssh password secret: %w", err)
	}
	return ref, nil
}

func hostSSHPasswordSecretRef(hostID string) string {
	trimmed := strings.TrimSpace(hostID)
	segment := safeSecretPathSegment(trimmed)
	if segment == "" {
		segment = "host"
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "secret://" + path.Join("hosts", segment+"-"+hex.EncodeToString(sum[:6]), "ssh-password")
}

func safeSecretPathSegment(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.', r == '@':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "._-")
}
