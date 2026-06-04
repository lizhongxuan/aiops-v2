package appui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSecretCredentialResolverRejectsPathTraversal(t *testing.T) {
	resolver := NewLocalSecretCredentialResolver(t.TempDir())

	if _, err := resolver.ResolveSSHCredential(context.Background(), "secret://../id_rsa"); err == nil {
		t.Fatal("ResolveSSHCredential() error = nil, want path traversal rejected")
	}
	if _, err := resolver.ResolveSSHCredential(context.Background(), "secret:///abs/key"); err == nil {
		t.Fatal("ResolveSSHCredential() error = nil, want absolute path rejected")
	}
	if _, err := resolver.ResolveSSHCredential(context.Background(), "file:///tmp/key"); err == nil {
		t.Fatal("ResolveSSHCredential() error = nil, want unsupported scheme rejected")
	}
}

func TestLocalSecretCredentialResolverWritesTempKey0600AndCleansUp(t *testing.T) {
	secretDir := t.TempDir()
	secretPath := filepath.Join(secretDir, "ops", "prod-web-01")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	key := "-----BEGIN OPENSSH PRIVATE KEY-----\nkey-material\n-----END OPENSSH PRIVATE KEY-----\n"
	if err := os.WriteFile(secretPath, []byte(key), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resolver := NewLocalSecretCredentialResolver(secretDir)
	credential, err := resolver.ResolveSSHCredential(context.Background(), "secret://ops/prod-web-01")
	if err != nil {
		t.Fatalf("ResolveSSHCredential() error = %v", err)
	}
	if credential.PrivateKeyPath == "" {
		t.Fatal("PrivateKeyPath is empty")
	}
	info, err := os.Stat(credential.PrivateKeyPath)
	if err != nil {
		t.Fatalf("Stat(temp key) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("temp key mode = %v, want 0600", got)
	}
	if credential.Password != "" {
		t.Fatal("Password should be empty for private key credential")
	}
	if err := credential.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(credential.PrivateKeyPath); !os.IsNotExist(err) {
		t.Fatalf("temp key after cleanup err = %v, want not exist", err)
	}
}

func TestLocalSecretCredentialResolverReadsPasswordWithoutTempKey(t *testing.T) {
	secretDir := t.TempDir()
	secretPath := filepath.Join(secretDir, "lab", "root-password")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(secretPath, []byte("s3cr3t-pass\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	resolver := NewLocalSecretCredentialResolver(secretDir)
	credential, err := resolver.ResolveSSHCredential(context.Background(), "secret://lab/root-password")
	if err != nil {
		t.Fatalf("ResolveSSHCredential() error = %v", err)
	}
	if credential.PrivateKeyPath != "" {
		t.Fatalf("PrivateKeyPath = %q, want empty for password credential", credential.PrivateKeyPath)
	}
	if credential.Password != "s3cr3t-pass" {
		t.Fatalf("Password = %q", credential.Password)
	}
	if err := credential.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
}

func TestLocalHostSSHPasswordStoreWritesPasswordForResolver(t *testing.T) {
	secretDir := t.TempDir()
	store := NewLocalHostSSHPasswordStore(secretDir)

	ref, err := store.StoreHostSSHPassword(context.Background(), "prod/web 01", "ssh-password-from-form")
	if err != nil {
		t.Fatalf("StoreHostSSHPassword() error = %v", err)
	}
	if !strings.HasPrefix(ref, "secret://hosts/prod_web_01-") {
		t.Fatalf("ref = %q, want generated host password secret ref", ref)
	}

	credential, err := NewLocalSecretCredentialResolver(secretDir).ResolveSSHCredential(context.Background(), ref)
	if err != nil {
		t.Fatalf("ResolveSSHCredential() error = %v", err)
	}
	if credential.Password != "ssh-password-from-form" {
		t.Fatalf("Password = %q", credential.Password)
	}

	rel, err := secretRefPath(ref)
	if err != nil {
		t.Fatalf("secretRefPath() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(secretDir, rel))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("secret file mode = %o, want 600", got)
	}
}

func TestLocalSecretCredentialResolverRedactsSecretMaterial(t *testing.T) {
	credential := ResolvedSSHCredential{
		Ref:            "secret://lab/root-password",
		PrivateKeyPath: "/tmp/aiops-key",
		Password:       "s3cr3t-pass",
	}
	text := credential.RedactedString()
	if strings.Contains(text, "s3cr3t-pass") || strings.Contains(text, "/tmp/aiops-key") {
		t.Fatalf("RedactedString() leaked secret material: %s", text)
	}
	if !strings.Contains(text, "secret://lab/root-password") {
		t.Fatalf("RedactedString() = %q, want safe ref", text)
	}
}
