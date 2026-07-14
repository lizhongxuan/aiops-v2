package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/integrations/localtools"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminalpolicy"
)

type fallbackHostCommandRunner struct {
	primary  localtools.HostAgentCommandRunner
	fallback localtools.HostAgentCommandRunner
}

func (r fallbackHostCommandRunner) RunHostAgentCommand(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	if r.primary == nil {
		if r.fallback == nil {
			return localtools.HostAgentCommandResult{}, fmt.Errorf("host command runner is not configured")
		}
		return r.fallback.RunHostAgentCommand(ctx, req)
	}
	result, err := r.primary.RunHostAgentCommand(ctx, req)
	if err == nil {
		return result, nil
	}
	if r.fallback == nil || !shouldFallbackHostCommandToSSH(err) {
		return localtools.HostAgentCommandResult{}, err
	}
	fallbackResult, fallbackErr := r.fallback.RunHostAgentCommand(ctx, req)
	if fallbackErr != nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("%v; ssh fallback failed: %w", err, fallbackErr)
	}
	return fallbackResult, nil
}

func shouldFallbackHostCommandToSSH(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{
		"not connected",
		"connection refused",
		"no such host",
		"does not have an agent url",
		"does not have a local host-agent token secret",
		"host-agent token resolver is not configured",
		"host-agent /exec status 404",
		"host-agent /run status 404",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

type sshCredentialResolver interface {
	ResolveSSHCredential(ctx context.Context, ref string) (appui.ResolvedSSHCredential, error)
}

type hostSSHCommandRunner struct {
	repo               localtools.HostRepository
	credentialResolver sshCredentialResolver
	executor           sshCommandExecutor
}

type sshCommandExecutor interface {
	RunSSHCommand(ctx context.Context, host store.HostRecord, credential appui.ResolvedSSHCredential, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error)
}

func (r hostSSHCommandRunner) RunHostAgentCommand(ctx context.Context, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	if !terminalpolicy.IsReadOnlyCommand(req.Command, req.Args) {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("ssh fallback requires a read-only command")
	}
	if r.repo == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host repository is not configured")
	}
	if r.credentialResolver == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("ssh credential resolver is not configured")
	}
	host, err := r.repo.GetHost(strings.TrimSpace(req.HostID))
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	if host == nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q not found", req.HostID)
	}
	if strings.TrimSpace(host.Address) == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q address is required for ssh fallback", req.HostID)
	}
	if strings.TrimSpace(host.SSHUser) == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q ssh user is required for ssh fallback", req.HostID)
	}
	credentialRef := strings.TrimSpace(host.SSHCredentialRef)
	if credentialRef == "" {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("host %q ssh credential ref is required for ssh fallback", req.HostID)
	}
	credential, err := r.credentialResolver.ResolveSSHCredential(ctx, credentialRef)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	if credential.Cleanup != nil {
		defer func() { _ = credential.Cleanup() }()
	}
	executor := r.executor
	if executor == nil {
		executor = goSSHCommandExecutor{}
	}
	result, err := executor.RunSSHCommand(ctx, *host, credential, req)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	if strings.TrimSpace(result.Source) == "" {
		result.Source = "host.ssh"
	}
	return result, nil
}

type goSSHCommandExecutor struct{}

func (goSSHCommandExecutor) RunSSHCommand(ctx context.Context, host store.HostRecord, credential appui.ResolvedSSHCredential, req localtools.HostAgentCommandRequest) (localtools.HostAgentCommandResult, error) {
	auth, err := sshAuthMethodsForCommandRunner(credential)
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	port := host.SSHPort
	if port <= 0 {
		port = 22
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	address := net.JoinHostPort(strings.TrimSpace(host.Address), strconv.Itoa(port))
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return localtools.HostAgentCommandResult{}, fmt.Errorf("ssh tcp connect failed: %w", err)
	}
	config := &ssh.ClientConfig{
		User:            strings.TrimSpace(host.SSHUser),
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return localtools.HostAgentCommandResult{}, fmt.Errorf("ssh authentication failed: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		return localtools.HostAgentCommandResult{}, err
	}
	defer func() { _ = session.Close() }()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	command := shellScriptForCommand(req.Command, req.Args, req.WorkingDir)
	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()
	exitCode := 0
	select {
	case <-ctx.Done():
		_ = session.Close()
		_ = client.Close()
		return localtools.HostAgentCommandResult{}, ctx.Err()
	case err := <-done:
		if err != nil {
			var exitErr *ssh.ExitError
			if !errors.As(err, &exitErr) {
				return localtools.HostAgentCommandResult{}, fmt.Errorf("ssh command failed: %w", err)
			}
			exitCode = exitErr.ExitStatus()
		}
	}
	return localtools.HostAgentCommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Source:   "host.ssh",
	}, nil
}

func sshAuthMethodsForCommandRunner(credential appui.ResolvedSSHCredential) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if path := strings.TrimSpace(credential.PrivateKeyPath); path != "" {
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read ssh private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if password := strings.TrimSpace(credential.Password); password != "" {
		methods = append(methods,
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = password
				}
				return answers, nil
			}),
		)
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("ssh credential ref is required")
	}
	return methods, nil
}
