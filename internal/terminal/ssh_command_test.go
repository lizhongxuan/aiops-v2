package terminal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHCommandFactoryBuildsSSHArgsWithoutSecretMaterial(t *testing.T) {
	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\nprivate-secret-material\n-----END OPENSSH PRIVATE KEY-----\n"
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd, err := BuildSSHCommand(SSHCommandRequest{
		Address: "10.0.0.11",
		User:    "ubuntu",
		Port:    2222,
		Credential: SSHCredential{
			PrivateKeyPath: keyPath,
		},
	})
	if err != nil {
		t.Fatalf("BuildSSHCommand() error = %v", err)
	}
	args := strings.Join(cmd.Args, "\x00")
	for _, want := range []string{"ssh", "-tt", "StrictHostKeyChecking=accept-new", "ServerAliveInterval=15", "-p", "2222", "-i", keyPath, "ubuntu@10.0.0.11"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q do not contain %q", args, want)
		}
	}
	if strings.Contains(args, "private-secret-material") {
		t.Fatalf("ssh args contain private key material: %q", args)
	}
}

func TestSSHCommandFactoryRejectsMissingCredentialRef(t *testing.T) {
	if _, err := BuildSSHCommand(SSHCommandRequest{
		Address: "10.0.0.11",
		User:    "ubuntu",
		Port:    22,
	}); err == nil {
		t.Fatalf("BuildSSHCommand() error = nil, want missing credential failure")
	}
}

func TestSSHCommandFactoryCleansTempKeyOnExit(t *testing.T) {
	binDir := t.TempDir()
	fakeSSH := filepath.Join(binDir, "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile(fake ssh) error = %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("dummy private key\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(key) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd, err := BuildSSHCommand(SSHCommandRequest{
		Address: "10.0.0.11",
		User:    "ubuntu",
		Port:    22,
		Credential: SSHCredential{
			PrivateKeyPath: keyPath,
			Cleanup: func() error {
				return os.Remove(keyPath)
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildSSHCommand() error = %v", err)
	}
	stdin, stdout, stderr, err := startCommand(cmd, "")
	if err != nil {
		t.Fatalf("startCommand() error = %v", err)
	}
	_ = stdin.Close()
	_, _ = io.ReadAll(stdout)
	_, _ = io.ReadAll(stderr)
	waitCommand(cmd)

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatalf("key path still exists after command exit, stat error = %v", err)
	}
}
