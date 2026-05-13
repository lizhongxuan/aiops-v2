package appui

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"aiops-v2/internal/store"
)

func TestNewServicesWithStorePersistsIncidents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "incidents.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite gorm db: %v", err)
	}
	dataStore, err := store.NewGormStore(db)
	if err != nil {
		t.Fatalf("NewGormStore() error = %v", err)
	}
	defer dataStore.Close()

	services := NewServices(runtimeStub{}, nil, WithStore(dataStore))
	created, err := services.IncidentService().Create(context.Background(), IncidentCreateCommand{
		Title:  "PG 集群异常",
		Source: "manual",
	})
	if err != nil {
		t.Fatalf("IncidentService.Create() error = %v", err)
	}

	persisted, ok := dataStore.GetIncident(created.ID)
	if !ok {
		t.Fatalf("GetIncident(%q) not found in store", created.ID)
	}
	if persisted.Title != "PG 集群异常" {
		t.Fatalf("persisted title = %q", persisted.Title)
	}
}
