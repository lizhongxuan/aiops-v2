package experiencepack

import (
	"context"
	"testing"
)

func TestMemoryStoreAppendOnlyAndIdempotent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	gene := testGene("gene_store")
	if err := store.AppendGene(ctx, "pack", gene); err != nil {
		t.Fatalf("append gene: %v", err)
	}
	if err := store.AppendGene(ctx, "pack", gene); err != nil {
		t.Fatalf("same asset and content should be idempotent: %v", err)
	}
	genes, err := store.ListGenes(ctx, "pack")
	if err != nil {
		t.Fatalf("list genes: %v", err)
	}
	if len(genes) != 1 {
		t.Fatalf("idempotent append should not duplicate, got %d", len(genes))
	}
}

func TestMemoryStoreDetectsSameAssetDifferentContentConflict(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	gene := testGene("gene_conflict")
	if err := store.AppendGene(ctx, "pack", gene); err != nil {
		t.Fatalf("append gene: %v", err)
	}
	changed := gene
	changed.Summary = "这是篡改后的不同内容，但 asset_id 被恶意复用"
	if err := store.AppendGene(ctx, "pack", changed); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestMemoryStoreRejectsMismatchedAssetID(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	gene := testGene("gene_bad_asset")
	gene.AssetID = MustHashCanonicalJSON(testGene("gene_other_asset"))
	if err := store.AppendGene(ctx, "pack", gene); err == nil {
		t.Fatal("expected mismatched asset_id to be rejected")
	}
}
