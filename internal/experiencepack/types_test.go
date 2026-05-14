package experiencepack

import (
	"encoding/json"
	"testing"
)

func TestGEPTypesValidateAndRoundTrip(t *testing.T) {
	gene := testGene("gene_pg")
	data, err := json.Marshal(gene)
	if err != nil {
		t.Fatalf("marshal gene: %v", err)
	}
	var decoded GEPGene
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal gene: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("gene should validate: %v", err)
	}

	capsule := testCapsule(gene.ID)
	data, err = json.Marshal(capsule)
	if err != nil {
		t.Fatalf("marshal capsule: %v", err)
	}
	var decodedCapsule GEPCapsule
	if err := json.Unmarshal(data, &decodedCapsule); err != nil {
		t.Fatalf("unmarshal capsule: %v", err)
	}
	if err := decodedCapsule.Validate(); err != nil {
		t.Fatalf("capsule should validate: %v", err)
	}
}

func TestRequiredFieldsFailValidation(t *testing.T) {
	gene := testGene("gene_missing")
	gene.SignalsMatch = nil
	if err := gene.Validate(); err == nil {
		t.Fatal("gene without signals_match should fail")
	}

	capsule := testCapsule("gene_missing")
	capsule.EnvFingerprint = EnvironmentFingerprint{}
	capsule.AssetID = MustHashCanonicalJSON(capsule)
	if err := capsule.Validate(); err == nil {
		t.Fatal("capsule without env_fingerprint should fail")
	}

	manifest := testManifest("pack_missing", testGene("gene_manifest"))
	manifest.Skill = SkillAsset{}
	manifest.Genes = nil
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	if err := manifest.Validate(); err == nil {
		t.Fatal("manifest without skill and gene should fail")
	}
}

func testGene(id string) GEPGene {
	gene := GEPGene{
		Type: "Gene", SchemaVersion: SchemaVersionGEP, ID: id, Category: CategoryInnovate,
		SignalsMatch: []string{"postgres|pg", "主从|replication", "/deploy.*pg/i"},
		Summary:      "部署 PostgreSQL 主从集群并配置 pg_mon",
		Strategy:     []string{"检查主机", "部署 primary", "部署 standby", "验证复制"},
		Constraints:  map[string]any{"max_files": 20, "forbidden_paths": []string{"/var/lib/pgsql/data when non-empty"}},
		Validation:   []string{"runner.readonly_probe:proof=proof-1"},
		Domain:       "aiops",
	}
	gene.AssetID = MustHashCanonicalJSON(gene)
	return gene
}

func testCapsule(geneID string) GEPCapsule {
	capsule := GEPCapsule{
		Type: "Capsule", SchemaVersion: SchemaVersionGEP, ID: "capsule_" + geneID,
		Trigger: []string{"postgres", "replication"}, Gene: geneID, GenesUsed: []string{geneID},
		Summary:    "成功部署 PostgreSQL 主从集群",
		Content:    "本次运维完成 PostgreSQL 主从集群部署、pg_mon 监控配置、复制验证和恢复证明，敏感主机和凭证均已脱敏。",
		Strategy:   []string{"检查主机", "部署 primary", "部署 standby", "验证复制"},
		Confidence: 0.9, BlastRadius: BlastRadius{Hosts: 3}, Outcome: Outcome{Status: "success", Score: 0.95},
		EnvFingerprint: EnvironmentFingerprint{OS: "linux", OSDistribution: "ubuntu", OSVersion: "22.04", PackageManager: "apt", HostCount: 3},
		Domain:         "aiops",
	}
	capsule.AssetID = MustHashCanonicalJSON(capsule)
	return capsule
}

func testManifest(id string, gene GEPGene) ExperiencePackManifest {
	skill := SkillAsset{Type: "Skill", Path: "skills/SKILL.md", Title: "PG 主从经验", Summary: "部署 PG 主从", Content: "# PG 主从经验\n"}
	skill.AssetID = HashSkillMarkdown(skill.Content)
	manifest := ExperiencePackManifest{
		Type: "AIOpsExperiencePack", SchemaVersion: SchemaVersionPack, ID: id, Name: "PG 主从经验",
		Summary: "部署 PG 主从", Category: CategoryInnovate, Status: PackStatusCandidate, ReviewStatus: PackStatusReviewPending,
		Skill: skill, RequiredFiles: requiredFiles(), Genes: []AssetRef{{ID: gene.ID, AssetID: gene.AssetID}},
	}
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	return manifest
}
