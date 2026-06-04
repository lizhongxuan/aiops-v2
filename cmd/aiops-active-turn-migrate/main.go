package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func main() {
	if err := runCLI(os.Args[1:], os.Stdout, os.Stderr, os.Getenv); err != nil {
		log.Fatalf("aiops-active-turn-migrate: %v", err)
	}
}

func runCLI(args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	fs := flag.NewFlagSet("aiops-active-turn-migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var dataDir, storeDriver, postgresDSN, mysqlDSN, output string
	var dryRun, apply bool
	fs.StringVar(&dataDir, "data-dir", firstNonEmpty(getenv("AIOPS_DATA_DIR"), ".data"), "AIOps data directory")
	fs.StringVar(&storeDriver, "store-driver", getenv("AIOPS_STORE_DRIVER"), "store driver: json, postgres, mysql")
	fs.StringVar(&postgresDSN, "postgres-dsn", firstNonEmpty(getenv("AIOPS_POSTGRES_DSN"), getenv("DATABASE_URL")), "Postgres DSN")
	fs.StringVar(&mysqlDSN, "mysql-dsn", getenv("AIOPS_MYSQL_DSN"), "MySQL DSN")
	fs.BoolVar(&dryRun, "dry-run", false, "only report active turn migration decisions")
	fs.BoolVar(&apply, "apply", false, "apply unrecoverable/manual-close migration marks")
	fs.StringVar(&output, "output", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if dryRun && apply {
		return fmt.Errorf("--apply and --dry-run cannot be used together")
	}
	if !dryRun && !apply {
		dryRun = true
	}
	output = strings.ToLower(strings.TrimSpace(output))
	if output == "" {
		output = "text"
	}
	if output != "text" && output != "json" {
		return fmt.Errorf("unsupported output format %q", output)
	}

	dataStore, err := store.OpenConfiguredStore(store.OpenConfig{
		DataDir:     dataDir,
		Driver:      storeDriver,
		PostgresDSN: postgresDSN,
		MySQLDSN:    mysqlDSN,
		FlushEvery:  time.Second,
	})
	if err != nil {
		return err
	}
	defer dataStore.Close()

	sessions, err := dataStore.ListSessions()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	now := time.Now().UTC()
	report := runtimekernel.BuildActiveTurnMigrationReport(
		fmt.Sprintf("active-turn-migration-%s", now.Format("20060102T150405Z")),
		sessions,
		runtimekernel.ActiveTurnMigrationOptions{
			Now:         now,
			DryRun:      dryRun,
			StoreDriver: normalizedStoreDriver(storeDriver),
		},
	)
	changed := 0
	if apply {
		for _, session := range sessions {
			sessionChanges := runtimekernel.ApplyActiveTurnMigration(session, report, runtimekernel.ActiveTurnMigrationApplyOptions{
				Now:    now,
				DryRun: false,
			})
			if sessionChanges == 0 {
				continue
			}
			if err := dataStore.SaveSession(session); err != nil {
				return fmt.Errorf("save migrated session %s: %w", session.ID, err)
			}
			changed += sessionChanges
		}
		if err := dataStore.Flush(); err != nil {
			return fmt.Errorf("flush migrated sessions: %w", err)
		}
		report.DryRun = false
	}

	switch output {
	case "json":
		payload := struct {
			runtimekernel.ActiveTurnMigrationReport
			Changed int `json:"changed"`
		}{
			ActiveTurnMigrationReport: report,
			Changed:                   changed,
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	default:
		_, err := fmt.Fprintf(stdout,
			"active-turn-migration dryRun=%t changed=%d total=%d resumable=%d requiresManualClose=%d unrecoverable=%d terminalNoop=%d\n",
			report.DryRun,
			changed,
			report.Summary.Total,
			report.Summary.Resumable,
			report.Summary.RequiresManualClose,
			report.Summary.Unrecoverable,
			report.Summary.TerminalNoop,
		)
		return err
	}
}

func normalizedStoreDriver(driver string) string {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		return "json"
	}
	return driver
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
