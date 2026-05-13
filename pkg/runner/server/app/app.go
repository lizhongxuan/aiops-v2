package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"
	"runner/logging"
	"runner/server/config"
	"runner/server/store/eventstore"
	"runner/server/ui"
)

var (
	Version   = "dev"
	BuildTime = "-"
)

type Options struct {
	ProgramName       string
	DefaultConfigPath string
	ForceUI           bool
}

type readinessChecker struct {
	cfg config.Config
}

func (c readinessChecker) Ready(_ *http.Request) error {
	dirs := []string{
		c.cfg.Stores.WorkflowsDir,
		c.cfg.Stores.ScriptsDir,
		c.cfg.Stores.SkillsDir,
		c.cfg.Stores.EnvironmentsDir,
		c.cfg.Stores.MCPDir,
		eventstore.DeriveRunEventDir(c.cfg.Stores.RunStateFile),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare dir %s: %w", dir, err)
		}
	}

	files := []string{
		c.cfg.Stores.RunStateFile,
		c.cfg.Stores.AgentStateFile,
	}
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return fmt.Errorf("prepare file dir %s: %w", filepath.Dir(file), err)
		}
	}

	if c.cfg.UI.Enabled {
		if err := ensureUIAssetsReady(c.cfg.UI.DistDir); err != nil {
			return err
		}
	}

	return nil
}

func Main(opts Options) {
	fs := flag.NewFlagSet(opts.ProgramName, flag.ExitOnError)
	configPath := fs.String("config", opts.DefaultConfigPath, "config file path")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse args: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	if opts.ForceUI {
		cfg.UI.Enabled = true
	}

	if _, err := logging.Init(logging.Config{
		LogLevel:  cfg.Logging.Level,
		LogFormat: cfg.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Config: cfg})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init runner runtime: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
		defer cancel()
		_ = runtime.Close(ctx)
	}()

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      runtime.Handler,
		ReadTimeout:  cfg.ReadTimeout(),
		WriteTimeout: cfg.WriteTimeout(),
	}

	logging.L().Info("runner server start",
		zap.String("program", opts.ProgramName),
		zap.String("addr", cfg.Server.Addr),
		zap.Bool("auth_enabled", cfg.Auth.Enabled),
		zap.Bool("ui_enabled", cfg.UI.Enabled),
		zap.String("ui_dist_dir", cfg.UI.DistDir),
	)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case sig := <-stop:
		logging.L().Info("runner server shutdown signal", zap.String("signal", sig.String()))
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logging.L().Error("runner server shutdown failed", zap.Error(err))
			os.Exit(1)
		}
		logging.L().Info("runner server stopped")
	case err := <-errCh:
		if err != nil {
			logging.L().Error("runner server failed", zap.Error(err))
			os.Exit(1)
		}
	}

	time.Sleep(50 * time.Millisecond)
}

func ensureUIAssetsReady(distDir string) error {
	if info, err := os.Stat(distDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("ui.dist_dir is not a directory: %s", distDir)
		}
		indexPath := filepath.Join(distDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			return nil
		}
	}

	embeddedUI, ok := ui.EmbeddedFS()
	if !ok {
		return fmt.Errorf("prepare ui dist dir %s: %w", distDir, os.ErrNotExist)
	}
	if _, err := fs.ReadFile(embeddedUI, "index.html"); err != nil {
		return fmt.Errorf("prepare embedded ui index: %w", err)
	}
	return nil
}
