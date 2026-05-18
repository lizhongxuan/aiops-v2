package opsmanual

import (
	"path/filepath"
	"testing"
)

func TestMemoryStoreClonesManualsCandidatesAndRunRecords(t *testing.T) {
	store := NewMemoryStore()
	manual := redisMemoryManual()
	manual.Tags = []string{"redis", "memory"}
	manual.RetrievalProfile = RetrievalProfile{Keywords: []string{"used_memory_rss"}}
	manual.PreflightProbe = PreflightProbe{ID: "redis_readonly_probe", RequiredOutputs: []string{"redis_ping"}}
	if err := store.SaveManual(manual); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	manual.Metadata = map[string]any{"mutated": true}
	manual.Tags[0] = "mutated"
	manual.RetrievalProfile.Keywords[0] = "mutated"
	manual.PreflightProbe.RequiredOutputs[0] = "mutated"
	restored, err := store.GetManual("manual-redis-memory")
	if err != nil {
		t.Fatalf("GetManual() error = %v", err)
	}
	if restored.Metadata["mutated"] != nil {
		t.Fatalf("store retained caller mutation: %#v", restored.Metadata)
	}
	if restored.Tags[0] != "redis" || restored.RetrievalProfile.Keywords[0] != "used_memory_rss" || restored.PreflightProbe.RequiredOutputs[0] != "redis_ping" {
		t.Fatalf("store retained nested caller mutation: %#v", restored)
	}

	candidate := ManualCandidate{ID: "candidate-1", ProposedManual: redisMemoryManual(), ReviewStatus: "pending"}
	if err := store.SaveCandidate(candidate); err != nil {
		t.Fatalf("SaveCandidate() error = %v", err)
	}
	candidates, err := store.ListCandidates()
	if err != nil || len(candidates) != 1 {
		t.Fatalf("ListCandidates() = %#v, %v", candidates, err)
	}

	record := RunRecord{ID: "record-1", ManualID: "manual-redis-memory", WorkflowID: "workflow-redis-memory", ValidationStatus: "passed", CompletedAt: "2026-05-14T08:00:00Z"}
	if err := store.SaveRunRecord(record); err != nil {
		t.Fatalf("SaveRunRecord() error = %v", err)
	}
	records, err := store.ListRunRecords(ListRunRecordsRequest{ManualID: "manual-redis-memory"})
	if err != nil || len(records) != 1 || records[0].ID != "record-1" {
		t.Fatalf("ListRunRecords() = %#v, %v", records, err)
	}
}

func TestFileStoreRoundTripsLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	manual := redisMemoryManual()
	manual.Tags = []string{"redis", "memory"}
	manual.RetrievalProfile = RetrievalProfile{Keywords: []string{"used_memory_rss"}}
	manual.PreflightProbe = PreflightProbe{ID: "redis_readonly_probe", RequiredOutputs: []string{"redis_ping"}}
	if err := store.SaveManual(manual); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	if err := store.SaveCandidate(ManualCandidate{ID: "candidate-1", ProposedManual: redisMemoryManual(), ReviewStatus: "pending"}); err != nil {
		t.Fatalf("SaveCandidate() error = %v", err)
	}
	if err := store.SaveRunRecord(RunRecord{ID: "record-1", ManualID: "manual-redis-memory", WorkflowID: "workflow-redis-memory", ValidationStatus: "passed"}); err != nil {
		t.Fatalf("SaveRunRecord() error = %v", err)
	}

	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reopen NewFileStore() error = %v", err)
	}
	manuals, err := reopened.ListManuals(ListManualsRequest{Status: ManualStatusVerified})
	if err != nil || len(manuals) != 1 {
		t.Fatalf("ListManuals() = %#v, %v", manuals, err)
	}
	if manuals[0].RetrievalProfile.Keywords[0] != "used_memory_rss" || manuals[0].PreflightProbe.ID != "redis_readonly_probe" {
		t.Fatalf("enhanced fields lost after file round trip: %#v", manuals[0])
	}
	candidates, err := reopened.ListCandidates()
	if err != nil || len(candidates) != 1 {
		t.Fatalf("ListCandidates() = %#v, %v", candidates, err)
	}
	records, err := reopened.ListRunRecords(ListRunRecordsRequest{ManualID: "manual-redis-memory"})
	if err != nil || len(records) != 1 {
		t.Fatalf("ListRunRecords() = %#v, %v", records, err)
	}
}

func TestFileStoreFixtureSupportsSearchOpsManualsDecisions(t *testing.T) {
	store, err := NewFileStore(filepath.Join("testdata", "search_ops_manuals_library.json"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}

	manuals, err := store.ListManuals(ListManualsRequest{Status: ManualStatusVerified})
	if err != nil {
		t.Fatalf("ListManuals() error = %v", err)
	}
	if len(manuals) != 4 {
		t.Fatalf("manual count = %d, want 4", len(manuals))
	}
	pgManual, err := store.GetManual("manual-pg-backup-ubuntu")
	if err != nil {
		t.Fatalf("GetManual(pg) error = %v", err)
	}
	assertManualFixture(t, pgManual)
	redisManual, err := store.GetManual("manual-redis-rca-ssh")
	if err != nil {
		t.Fatalf("GetManual(redis) error = %v", err)
	}
	if redisManual.Title != "Redis SSH 排障运维手册" {
		t.Fatalf("redis title = %q", redisManual.Title)
	}
	if !hasAny(redisManual.RequiredContext.RequiredInputs, "target_instance") || !hasAny(redisManual.RequiredContext.RequiredEvidence, "symptom", "metrics") {
		t.Fatalf("redis required context = %#v", redisManual.RequiredContext)
	}

	records, err := store.ListRunRecords(ListRunRecordsRequest{ManualID: "manual-pg-backup-ubuntu", WorkflowID: "workflow-pg-backup-ubuntu"})
	if err != nil {
		t.Fatalf("ListRunRecords(pg) error = %v", err)
	}
	if len(records) != 1 || records[0].ExecutionStatus != "passed" || records[0].ValidationStatus != "passed" {
		t.Fatalf("pg records = %#v, want one passed record", records)
	}

	direct := searchFixtureWithMetadata(t, store, "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"})
	if direct.Decision != DecisionDirectExecute {
		t.Fatalf("direct decision = %q, want direct_execute; result=%#v", direct.Decision, direct)
	}
	if len(direct.Manuals) == 0 || direct.Manuals[0].Manual.ID != "manual-pg-backup-ubuntu" {
		t.Fatalf("direct manuals = %#v, want pg manual first", direct.Manuals)
	}
	if direct.Manuals[0].RunRecordSummary.SuccessCount != 1 || direct.Manuals[0].RunRecordSummary.RecentResult != "passed" {
		t.Fatalf("direct run summary = %#v, want successful fixture run", direct.Manuals[0].RunRecordSummary)
	}

	adapt := searchFixtureWithMetadata(t, store, "在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-centos-01"})
	if adapt.Decision != DecisionAdapt {
		t.Fatalf("adapt decision = %q, want adapt; result=%#v", adapt.Decision, adapt)
	}
	if len(adapt.Manuals) == 0 || !hasAny(adapt.Manuals[0].EnvironmentDiffs, "os") || !hasAny(adapt.Manuals[0].EnvironmentDiffs, "package_manager") {
		t.Fatalf("adapt diffs = %#v, want os and package_manager", adapt.Manuals)
	}

	needInfo := searchFixture(t, store, "排查 Redis")
	if needInfo.Decision != DecisionNeedInfo {
		t.Fatalf("need info decision = %q, want need_info; result=%#v", needInfo.Decision, needInfo)
	}
	if len(needInfo.Manuals) == 0 || needInfo.Manuals[0].Manual.ID != "manual-redis-rca-ssh" {
		t.Fatalf("need info manuals = %#v, want redis manual first", needInfo.Manuals)
	}
	for _, want := range []string{"target_instance", "execution_surface", "symptom", "metrics"} {
		if !hasAny(needInfo.Manuals[0].MissingFields, want) {
			t.Fatalf("missing fields = %#v, want %q", needInfo.Manuals[0].MissingFields, want)
		}
	}
}

func assertManualFixture(t *testing.T, manual OpsManual) {
	t.Helper()
	if manual.Title != "PostgreSQL 备份 Ubuntu 运维手册" {
		t.Fatalf("pg title = %q", manual.Title)
	}
	if manual.Status != ManualStatusVerified {
		t.Fatalf("pg status = %q, want verified", manual.Status)
	}
	if manual.WorkflowRef.WorkflowID != "workflow-pg-backup-ubuntu" {
		t.Fatalf("pg workflow = %q", manual.WorkflowRef.WorkflowID)
	}
	if manual.Operation.TargetType != "postgresql" || manual.Operation.Action != "backup" {
		t.Fatalf("pg operation = %#v", manual.Operation)
	}
	if !hasAny(manual.Applicability.OS, "ubuntu") || !hasAny(manual.Applicability.ExecutionSurface, "ssh") {
		t.Fatalf("pg applicability = %#v", manual.Applicability)
	}
	if !hasAny(manual.RequiredContext.RequiredInputs, "target_instance") || !hasAny(manual.RequiredContext.RequiredInputs, "backup_path") {
		t.Fatalf("pg required inputs = %#v", manual.RequiredContext.RequiredInputs)
	}
	if !hasAny(manual.RequiredContext.RequiredEvidence, "ssh_access") || !hasAny(manual.RequiredContext.RequiredEvidence, "pg_isready") {
		t.Fatalf("pg required evidence = %#v", manual.RequiredContext.RequiredEvidence)
	}
	if !hasValidationContaining(manual.Validation, "pg_isready") || !hasValidationContaining(manual.Validation, "backup file exists") {
		t.Fatalf("pg validation = %#v", manual.Validation)
	}
}

func searchFixture(t *testing.T, store *FileStore, text string) SearchOpsManualsResult {
	t.Helper()
	return searchFixtureWithMetadata(t, store, text, nil)
}

func searchFixtureWithMetadata(t *testing.T, store *FileStore, text string, metadata map[string]any) SearchOpsManualsResult {
	t.Helper()
	result, err := SearchOpsManuals(store, SearchOpsManualsRequest{Text: text, Metadata: metadata})
	if err != nil {
		t.Fatalf("SearchOpsManuals(%q) error = %v", text, err)
	}
	return result
}

func hasValidationContaining(values []string, want string) bool {
	for _, value := range values {
		if stringsContains(value, want) {
			return true
		}
	}
	return false
}
