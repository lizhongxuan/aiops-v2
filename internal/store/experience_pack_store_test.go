package store

import (
	"context"
	"errors"
	"testing"

	"aiops-v2/internal/experiencepack"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGormExperiencePackStorePersistsBundle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dataStore, err := NewGormStore(db)
	if err != nil {
		t.Fatalf("new gorm store: %v", err)
	}
	ctx := context.Background()
	svc := experiencepack.NewService(dataStore)
	bundle, err := svc.GenerateAndPersistCandidate(ctx, experiencepack.CandidateInput{
		PackID: "pack_store_pg", Name: "PG Store Fixture", Summary: "Persist PG experience pack",
		Trajectory: experiencepack.Trajectory{CaseID: "case-store", UserGoal: "部署 pg 主从", ProofID: "proof", Outcome: "success", Environment: experiencepack.EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", PackageManager: "apt", HostCount: 3}},
	})
	if err != nil {
		t.Fatalf("persist candidate: %v", err)
	}
	manifest, err := dataStore.GetManifest(ctx, bundle.Manifest.ID, "")
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if manifest.ID != "pack_store_pg" {
		t.Fatalf("manifest id=%q", manifest.ID)
	}
	genes, err := dataStore.ListGenes(ctx, manifest.ID)
	if err != nil || len(genes) != 1 {
		t.Fatalf("genes len=%d err=%v", len(genes), err)
	}
	capsules, err := dataStore.ListCapsules(ctx, manifest.ID)
	if err != nil || len(capsules) != 1 {
		t.Fatalf("capsules len=%d err=%v", len(capsules), err)
	}
}

func TestGormExperiencePackStoreDetectsAssetConflict(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dataStore, err := NewGormStore(db)
	if err != nil {
		t.Fatalf("new gorm store: %v", err)
	}
	ctx := context.Background()
	gene := testStoreGene("gene_conflict")
	if err := dataStore.AppendGene(ctx, "pack", gene); err != nil {
		t.Fatalf("append gene: %v", err)
	}
	changed := gene
	changed.Summary = "changed summary with reused asset id"
	if err := dataStore.AppendGene(ctx, "pack", changed); !errors.Is(err, experiencepack.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestGormExperiencePackStoreRejectsMismatchedAssetID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dataStore, err := NewGormStore(db)
	if err != nil {
		t.Fatalf("new gorm store: %v", err)
	}
	gene := testStoreGene("gene_bad_asset")
	gene.AssetID = experiencepack.MustHashCanonicalJSON(testStoreGene("gene_other_asset"))
	if err := dataStore.AppendGene(context.Background(), "pack", gene); err == nil {
		t.Fatal("expected mismatched asset_id to be rejected")
	}
}

func testStoreGene(id string) experiencepack.GEPGene {
	gene := experiencepack.GEPGene{
		Type: "Gene", SchemaVersion: experiencepack.SchemaVersionGEP, ID: id, Category: experiencepack.CategoryInnovate,
		SignalsMatch: []string{"postgres", "主从", "deploy"},
		Summary:      "部署 PostgreSQL 主从集群",
		Strategy:     []string{"check", "deploy", "validate"},
		Constraints:  map[string]any{"max_files": 20, "forbidden_paths": []string{"/var/lib/pgsql/data"}},
		Validation:   []string{"runner.readonly_probe:proof=proof"},
	}
	gene.AssetID = experiencepack.MustHashCanonicalJSON(gene)
	return gene
}
