package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type SSHCredential struct {
	PrivateKeyPath string
	Password       string
	Cleanup        func() error
}

type SSHCommandRequest struct {
	HostID     string
	Address    string
	User       string
	Port       int
	Credential SSHCredential
}

var commandCleanups sync.Map

func BuildSSHCommand(req SSHCommandRequest) (*exec.Cmd, error) {
	address := strings.TrimSpace(req.Address)
	if address == "" {
		return nil, fmt.Errorf("host address is required")
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		return nil, fmt.Errorf("ssh user is required")
	}
	port := req.Port
	if port <= 0 {
		port = 22
	}
	keyPath := strings.TrimSpace(req.Credential.PrivateKeyPath)
	password := strings.TrimSpace(req.Credential.Password)
	if keyPath == "" && password == "" {
		return nil, fmt.Errorf("ssh credential ref is required")
	}

	args := []string{
		"-tt",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=15",
		"-p", strconv.Itoa(port),
	}
	env := append(os.Environ(), "TERM=xterm-256color")
	cleanup := req.Credential.Cleanup

	if keyPath != "" {
		if err := os.Chmod(keyPath, 0o600); err != nil {
			return nil, fmt.Errorf("chmod ssh key: %w", err)
		}
		args = append(args, "-i", keyPath, "-o", "IdentitiesOnly=yes")
	} else {
		askpassPath, err := writeAskpassScript()
		if err != nil {
			if cleanup != nil {
				_ = cleanup()
			}
			return nil, err
		}
		env = append(env,
			"SSH_ASKPASS="+askpassPath,
			"SSH_ASKPASS_REQUIRE=force",
			"DISPLAY=none",
			"AIOPS_SSH_PASSWORD="+password,
		)
		args = append(args,
			"-o", "PreferredAuthentications=password,keyboard-interactive,publickey",
			"-o", "PubkeyAuthentication=no",
		)
		cleanup = joinCleanup(cleanup, func() error { return os.Remove(askpassPath) })
	}
	args = append(args, user+"@"+address)

	cmd := exec.Command("ssh", args...)
	cmd.Env = env
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if cleanup != nil {
		registerCommandCleanup(cmd, cleanup)
	}
	return cmd, nil
}

func writeAskpassScript() (string, error) {
	file, err := os.CreateTemp("", "aiops-ssh-askpass-*")
	if err != nil {
		return "", fmt.Errorf("create askpass script: %w", err)
	}
	path := file.Name()
	if _, err := file.WriteString("#!/bin/sh\nprintf '%s\\n' \"$AIOPS_SSH_PASSWORD\"\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write askpass script: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close askpass script: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod askpass script: %w", err)
	}
	return path, nil
}

func joinCleanup(first, second func() error) func() error {
	return func() error {
		var err error
		if first != nil {
			err = first()
		}
		if second != nil {
			if secondErr := second(); err == nil {
				err = secondErr
			}
		}
		return err
	}
}

func registerCommandCleanup(cmd *exec.Cmd, cleanup func() error) {
	if cmd == nil || cleanup == nil {
		return
	}
	commandCleanups.Store(cmd, cleanup)
}

func runCommandCleanup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	value, ok := commandCleanups.LoadAndDelete(cmd)
	if !ok {
		return
	}
	if cleanup, ok := value.(func() error); ok {
		_ = cleanup()
	}
}

func waitCommand(cmd *exec.Cmd) (int, string) {
	defer runCommandCleanup(cmd)
	if cmd == nil {
		return 0, ""
	}
	if err := cmd.Wait(); err != nil {
		return parseExitError(err)
	}
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), ""
	}
	return 0, ""
}
