package appui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ResolvedSSHCredential struct {
	Ref            string
	PrivateKeyPath string
	Password       string
	Cleanup        func() error
}

func (c ResolvedSSHCredential) RedactedString() string {
	return fmt.Sprintf("ssh credential ref=%s privateKeyPath=[redacted] password=[redacted]", c.Ref)
}

type CredentialResolver interface {
	ResolveSSHCredential(ctx context.Context, ref string) (ResolvedSSHCredential, error)
}

type localSecretCredentialResolver struct {
	secretDir string
}

func NewLocalSecretCredentialResolver(secretDir string) CredentialResolver {
	return localSecretCredentialResolver{secretDir: strings.TrimSpace(secretDir)}
}

func (r localSecretCredentialResolver) ResolveSSHCredential(ctx context.Context, ref string) (ResolvedSSHCredential, error) {
	select {
	case <-ctx.Done():
		return ResolvedSSHCredential{}, ctx.Err()
	default:
	}
	rel, err := secretRefPath(ref)
	if err != nil {
		return ResolvedSSHCredential{}, err
	}
	if strings.TrimSpace(r.secretDir) == "" {
		return ResolvedSSHCredential{}, fmt.Errorf("secret directory is not configured")
	}
	source := filepath.Join(r.secretDir, rel)
	cleanSecretDir, err := filepath.Abs(r.secretDir)
	if err != nil {
		return ResolvedSSHCredential{}, err
	}
	cleanSource, err := filepath.Abs(source)
	if err != nil {
		return ResolvedSSHCredential{}, err
	}
	if !strings.HasPrefix(cleanSource, cleanSecretDir+string(os.PathSeparator)) && cleanSource != cleanSecretDir {
		return ResolvedSSHCredential{}, fmt.Errorf("secret ref escapes secret directory")
	}
	content, err := os.ReadFile(cleanSource)
	if err != nil {
		return ResolvedSSHCredential{}, fmt.Errorf("read ssh credential %s: %w", ref, err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return ResolvedSSHCredential{}, fmt.Errorf("ssh credential %s is empty", ref)
	}
	if looksLikePrivateKey(trimmed) {
		return writeTempPrivateKey(ref, []byte(trimmed+"\n"))
	}
	return ResolvedSSHCredential{
		Ref:      strings.TrimSpace(ref),
		Password: trimmed,
		Cleanup:  func() error { return nil },
	}, nil
}

func secretRefPath(ref string) (string, error) {
	const prefix = "secret://"
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, prefix) {
		return "", fmt.Errorf("unsupported credential ref")
	}
	rel := strings.TrimPrefix(trimmed, prefix)
	rel = strings.TrimLeft(rel, "/")
	if rel == "" {
		return "", fmt.Errorf("credential ref path is required")
	}
	if strings.Contains(rel, "\\") {
		return "", fmt.Errorf("credential ref path must use forward slashes")
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", fmt.Errorf("credential ref path is invalid")
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", fmt.Errorf("credential ref path is invalid")
	}
	return clean, nil
}

func looksLikePrivateKey(content string) bool {
	return strings.Contains(content, "BEGIN OPENSSH PRIVATE KEY") ||
		strings.Contains(content, "BEGIN RSA PRIVATE KEY") ||
		strings.Contains(content, "BEGIN EC PRIVATE KEY") ||
		strings.Contains(content, "BEGIN PRIVATE KEY")
}

func writeTempPrivateKey(ref string, content []byte) (ResolvedSSHCredential, error) {
	file, err := os.CreateTemp("", "aiops-ssh-key-*")
	if err != nil {
		return ResolvedSSHCredential{}, fmt.Errorf("create temporary ssh key: %w", err)
	}
	path := file.Name()
	cleanup := func() error {
		return os.Remove(path)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = cleanup()
		return ResolvedSSHCredential{}, fmt.Errorf("chmod temporary ssh key: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = cleanup()
		return ResolvedSSHCredential{}, fmt.Errorf("write temporary ssh key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = cleanup()
		return ResolvedSSHCredential{}, fmt.Errorf("close temporary ssh key: %w", err)
	}
	return ResolvedSSHCredential{
		Ref:            strings.TrimSpace(ref),
		PrivateKeyPath: path,
		Cleanup:        cleanup,
	}, nil
}
