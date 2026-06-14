package store

import (
	"fmt"
	"strings"
	"time"
)

type OpenConfig struct {
	DataDir     string
	Driver      string
	PostgresDSN string
	MySQLDSN    string
	FlushEvery  time.Duration
}

func OpenConfiguredStore(cfg OpenConfig) (Store, error) {
	flushEvery := cfg.FlushEvery
	if flushEvery <= 0 {
		flushEvery = 5 * time.Second
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	switch driver {
	case "", "json", "file":
		return NewJSONFileStore(cfg.DataDir, flushEvery)
	case "postgres", "postgresql":
		dsn := strings.TrimSpace(cfg.PostgresDSN)
		if dsn == "" {
			return nil, fmt.Errorf("AIOPS_POSTGRES_DSN is required when AIOPS_STORE_DRIVER=postgres")
		}
		return NewPostgresStore(dsn)
	case "mysql":
		dsn := strings.TrimSpace(cfg.MySQLDSN)
		if dsn == "" {
			return nil, fmt.Errorf("AIOPS_MYSQL_DSN is required when AIOPS_STORE_DRIVER=mysql")
		}
		return NewMySQLStore(dsn)
	default:
		return nil, fmt.Errorf("unsupported store driver %q", driver)
	}
}

func OpenConfigFromEnv(dataDir string, getenv func(string) string) OpenConfig {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	postgresDSN := strings.TrimSpace(getenv("AIOPS_POSTGRES_DSN"))
	if postgresDSN == "" {
		postgresDSN = strings.TrimSpace(getenv("DATABASE_URL"))
	}
	return OpenConfig{
		DataDir:     dataDir,
		Driver:      getenv("AIOPS_STORE_DRIVER"),
		PostgresDSN: postgresDSN,
		MySQLDSN:    getenv("AIOPS_MYSQL_DSN"),
		FlushEvery:  5 * time.Second,
	}
}
