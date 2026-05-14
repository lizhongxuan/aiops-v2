package appui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestExperiencePackServiceReviewAndValidationGates(t *testing.T) {
	service := NewExperiencePackService(nil)

	candidate, err := service.PrepareCandidate(ExperiencePackPrepareCandidateRequest{
		CaseID: "case-1",
		PackID: "pack-1",
		Title:  "Checkout lock wait",
	})
	if err != nil {
		t.Fatalf("prepare candidate: %v", err)
	}

	if _, err := service.RetrieveCandidate(candidate.ID); !errors.Is(err, ErrExperiencePackCandidateNotApproved) {
		t.Fatalf("retrieve before approval error=%v want %v", err, ErrExperiencePackCandidateNotApproved)
	}

	pack, err := service.ConfirmCandidate(candidate.ID, ExperiencePackReviewRequest{Reviewer: "sre", Decision: "approve"})
	if err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if pack.ReviewStatus != "approved" {
		t.Fatalf("review status=%q want approved", pack.ReviewStatus)
	}
	if !pack.Enabled || pack.Status != "enabled" {
		t.Fatalf("approved pack enabled=%v status=%q want enabled", pack.Enabled, pack.Status)
	}
	if len(pack.AuthorizationScopes) != 1 || !pack.AuthorizationScopes[0].Searchable {
		t.Fatalf("approved pack scopes=%+v want default searchable scope", pack.AuthorizationScopes)
	}

	blocked := pack
	blocked.ValidationGate.Status = "blocked"
	raw := service.(*defaultExperiencePackService)
	if _, err := raw.savePack(blocked); err != nil {
		t.Fatalf("save blocked pack: %v", err)
	}

	if _, err := service.SetPackEnabled(pack.ID, true, ExperiencePackReviewRequest{Reviewer: "sre"}); !errors.Is(err, ErrExperiencePackValidationBlocked) {
		t.Fatalf("enable blocked pack error=%v want %v", err, ErrExperiencePackValidationBlocked)
	}

	paused, err := service.SetPackEnabled(pack.ID, false, ExperiencePackReviewRequest{Reviewer: "sre"})
	if err != nil {
		t.Fatalf("pause blocked pack: %v", err)
	}
	if paused.Enabled || paused.Status != "paused" {
		t.Fatalf("paused pack enabled=%v status=%q want paused", paused.Enabled, paused.Status)
	}
}

func TestExperiencePackServiceRunnerCandidateCreatesDraftBinding(t *testing.T) {
	service := NewExperiencePackService(nil)
	candidate, err := service.PrepareCandidate(ExperiencePackPrepareCandidateRequest{
		CaseID:  "case-pg-1",
		PackID:  "pack-pg-1",
		Title:   "PG 主从部署",
		Summary: "在 A/B/C 主机部署 PG 主从和 pg_mon",
	})
	if err != nil {
		t.Fatalf("prepare experience candidate: %v", err)
	}
	if _, err := service.ConfirmCandidate(candidate.ID, ExperiencePackReviewRequest{Reviewer: "sre", Decision: "approve"}); err != nil {
		t.Fatalf("confirm experience candidate: %v", err)
	}

	runner, err := service.ConfirmRunnerCandidate(ExperiencePackRunnerCandidateRequest{
		PackID: "pack-pg-1",
		Commands: []string{
			"install pg on xxA",
			"install pg on xxB",
			"configure primary standby",
			"install pg_mon on xxC",
			"verify replication",
			"verify pg_mon",
		},
	})
	if err != nil {
		t.Fatalf("confirm runner candidate: %v", err)
	}
	if !strings.HasPrefix(runner.StudioDraftLink, "/runner/") {
		t.Fatalf("studio draft link=%q want /runner/ prefix", runner.StudioDraftLink)
	}
	if runner.WorkflowID == "" || runner.Workflow["graph"] == nil {
		t.Fatalf("runner draft missing workflow graph: %+v", runner)
	}
	graph := runner.Graph
	nodes, ok := graph["nodes"].([]map[string]any)
	if !ok {
		t.Fatalf("graph nodes = %#v, want []map[string]any", graph["nodes"])
	}
	var approvalNode map[string]any
	for _, node := range nodes {
		if node["id"] == "approval" {
			approvalNode = node
			break
		}
	}
	if approvalNode == nil {
		t.Fatalf("graph nodes = %#v, want approval node", nodes)
	}
	if approvalNode["type"] != "manual_approval" {
		t.Fatalf("approval node type=%q want manual_approval", approvalNode["type"])
	}
	approvalSpec, ok := approvalNode["approval"].(map[string]any)
	if !ok {
		t.Fatalf("approval node missing top-level approval spec: %#v", approvalNode)
	}
	if approvalSpec["timeout"] != "30m" || approvalSpec["on_timeout"] != "reject" {
		t.Fatalf("approval spec = %#v, want timeout and reject policy", approvalSpec)
	}
	edges, ok := graph["edges"].([]map[string]any)
	if !ok {
		t.Fatalf("graph edges = %#v, want []map[string]any", graph["edges"])
	}
	kinds := map[string]bool{}
	for _, edge := range edges {
		kinds[edge["kind"].(string)] = true
		if edge["kind"] == "approved" || edge["kind"] == "rejected" {
			t.Fatalf("edge %v uses unsupported approval kind %q", edge["id"], edge["kind"])
		}
	}
	if !kinds["approval_approved"] || !kinds["approval_rejected"] {
		t.Fatalf("edge kinds=%v want approval_approved and approval_rejected", kinds)
	}
	pack, err := service.GetPack("pack-pg-1")
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	if len(pack.RunnerBindings) != 1 {
		t.Fatalf("runner bindings=%d want 1", len(pack.RunnerBindings))
	}
	if pack.RunnerBindings[0].Status != "draft" || pack.RunnerBindings[0].ReviewStatus != "pending" {
		t.Fatalf("binding should start as draft/pending: %+v", pack.RunnerBindings[0])
	}
}

func TestExperiencePackServiceListCandidatesReflectsLatestPackState(t *testing.T) {
	service := NewExperiencePackService(nil)
	candidate, err := service.PrepareCandidate(ExperiencePackPrepareCandidateRequest{
		CaseID: "case-1",
		PackID: "pack-1",
		Title:  "Checkout lock wait",
	})
	if err != nil {
		t.Fatalf("prepare candidate: %v", err)
	}
	pack, err := service.ConfirmCandidate(candidate.ID, ExperiencePackReviewRequest{Reviewer: "sre", Decision: "approve"})
	if err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if _, err := service.SaveAuthorizationScopes(pack.ID, []ExperiencePackAuthorizationScope{{
		Type:       "environment",
		Value:      "prod/postgres",
		Searchable: true,
	}}); err != nil {
		t.Fatalf("save scopes: %v", err)
	}
	if _, err := service.EnablePack(pack.ID, ExperiencePackReviewRequest{Reviewer: "sre"}); err != nil {
		t.Fatalf("enable pack: %v", err)
	}

	list, err := service.ListCandidates(ListExperiencePackCandidatesRequest{Limit: 100})
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ExperiencePack == nil {
		t.Fatalf("candidate list missing embedded pack: %+v", list.Items)
	}
	got := list.Items[0].ExperiencePack
	if got.Status != "enabled" || !got.Enabled || len(got.AuthorizationScopes) != 1 || !got.AuthorizationScopes[0].Searchable {
		t.Fatalf("embedded pack not refreshed from latest state: %+v", got)
	}
}

func TestExperiencePackServicePersistsLibraryStateWithFileRepository(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "experience-packs", "library.json")
	service := NewExperiencePackService(NewFileExperiencePackRepository(repoPath))
	candidate, err := service.PrepareCandidate(ExperiencePackPrepareCandidateRequest{
		Title:       "Postgres 主从部署经验",
		Summary:     "部署 pg 主从并验证复制状态",
		Service:     "postgres",
		Environment: "prod",
	})
	if err != nil {
		t.Fatalf("prepare candidate: %v", err)
	}
	pack, err := service.ConfirmCandidate(candidate.ID, ExperiencePackReviewRequest{Reviewer: "sre", Decision: "approve"})
	if err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if _, err := service.SaveAuthorizationScopes(pack.ID, []ExperiencePackAuthorizationScope{{Type: "environment", Value: "prod"}}); err != nil {
		t.Fatalf("save scopes: %v", err)
	}
	if _, err := service.EnablePack(pack.ID, ExperiencePackReviewRequest{Reviewer: "sre"}); err != nil {
		t.Fatalf("enable pack: %v", err)
	}

	restored := NewExperiencePackService(NewFileExperiencePackRepository(repoPath))
	list, err := restored.ListPacks(ListExperiencePacksRequest{})
	if err != nil {
		t.Fatalf("list restored packs: %v", err)
	}
	if len(list.Items) != 1 || !list.Items[0].Enabled || len(list.Items[0].AuthorizationScopes) != 1 {
		t.Fatalf("restored packs = %+v, want enabled pack with scope", list.Items)
	}
	matches, err := restored.Retrieve(ExperiencePackRetrieveRequest{UserText: "postgres pg 主从"})
	if err != nil {
		t.Fatalf("retrieve restored pack: %v", err)
	}
	if matches.Total != 1 || matches.Items[0].PackID != pack.ID {
		t.Fatalf("matches = %+v, want restored pack %q", matches, pack.ID)
	}
	longMatches, err := restored.Retrieve(ExperiencePackRetrieveRequest{
		UserText: "请复用经验包部署 PostgreSQL 16.13 主从，pg_mon 放主机C，输出很多 Markdown、审计项和回滚说明，最终 outcome=success。",
	})
	if err != nil {
		t.Fatalf("retrieve long text: %v", err)
	}
	if len(longMatches.Items) != 1 {
		t.Fatalf("long matches = %+v, want one match", longMatches)
	}
	if got := len(longMatches.Items[0].MatchedSignals); got > 12 {
		t.Fatalf("matched signals len=%d signals=%+v, want compact list", got, longMatches.Items[0].MatchedSignals)
	}
}

func TestExperiencePackServiceDelegatesRetrieveToRepositoryIndex(t *testing.T) {
	repo := &indexedExperiencePackRepoStub{
		matchList: ExperiencePackMatchList{
			Items: []ExperiencePackMatch{{
				PackID:         "pack-indexed-pg",
				Skill:          ExperiencePackSkill{Name: "PG 主从部署经验"},
				Confidence:     0.92,
				MatchedSignals: []string{"postgresql", "pg_mon"},
				MatchReasons:   []string{"pgvector 语义索引命中"},
			}},
			Total: 1,
		},
	}
	service := NewExperiencePackService(repo)
	matches, err := service.Retrieve(ExperiencePackRetrieveRequest{UserText: "部署 PostgreSQL 主从，pg_mon 在 C"})
	if err != nil {
		t.Fatalf("retrieve via indexed repo: %v", err)
	}
	if !repo.retrieveCalled {
		t.Fatal("repository indexed retrieve was not called")
	}
	if matches.Total != 1 || matches.Items[0].PackID != "pack-indexed-pg" || matches.Items[0].Confidence != 0.92 {
		t.Fatalf("matches = %+v, want indexed repo result", matches)
	}
}

type indexedExperiencePackRepoStub struct {
	matchList      ExperiencePackMatchList
	retrieveCalled bool
}

func (r *indexedExperiencePackRepoStub) ListExperiencePacks(ListExperiencePacksRequest) (ExperiencePackLibraryList, error) {
	return ExperiencePackLibraryList{}, nil
}
func (r *indexedExperiencePackRepoStub) ListExperiencePackCandidates(ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error) {
	return ExperiencePackCandidateList{}, nil
}
func (r *indexedExperiencePackRepoStub) SaveExperiencePackCandidate(ExperiencePackCandidate) error {
	return nil
}
func (r *indexedExperiencePackRepoStub) GetExperiencePackCandidate(string) (ExperiencePackCandidate, error) {
	return ExperiencePackCandidate{}, ErrExperiencePackCandidateNotFound
}
func (r *indexedExperiencePackRepoStub) GetExperiencePack(string) (ExperiencePack, error) {
	return ExperiencePack{}, ErrExperiencePackNotFound
}
func (r *indexedExperiencePackRepoStub) SaveExperiencePack(ExperiencePack) error { return nil }
func (r *indexedExperiencePackRepoStub) ListExperiencePackReuseRecords(string, ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error) {
	return ExperiencePackReuseRecordList{}, nil
}
func (r *indexedExperiencePackRepoStub) RetrieveExperiencePacks(req ExperiencePackRetrieveRequest) (ExperiencePackMatchList, error) {
	r.retrieveCalled = true
	return r.matchList, nil
}

func TestExperiencePackServiceRetrieveOnlyOffersDryRunForPublishedRunnerBinding(t *testing.T) {
	service := NewExperiencePackService(nil)
	candidate, err := service.PrepareCandidate(ExperiencePackPrepareCandidateRequest{
		CaseID: "case-1",
		PackID: "pack-1",
		Title:  "Checkout lock wait",
	})
	if err != nil {
		t.Fatalf("prepare candidate: %v", err)
	}
	pack, err := service.ConfirmCandidate(candidate.ID, ExperiencePackReviewRequest{Reviewer: "sre", Decision: "approve"})
	if err != nil {
		t.Fatalf("confirm candidate: %v", err)
	}
	if _, err := service.EnablePack(pack.ID, ExperiencePackReviewRequest{Reviewer: "sre"}); err != nil {
		t.Fatalf("enable pack: %v", err)
	}
	if _, err := service.ConfirmRunnerCandidate(ExperiencePackRunnerCandidateRequest{PackID: pack.ID}); err != nil {
		t.Fatalf("confirm runner candidate: %v", err)
	}

	matches, err := service.Retrieve(ExperiencePackRetrieveRequest{UserText: "checkout"})
	if err != nil {
		t.Fatalf("retrieve with draft binding: %v", err)
	}
	if len(matches.Items) != 1 {
		t.Fatalf("matches=%d want 1", len(matches.Items))
	}
	if containsExperiencePackAction(matches.Items[0].NextActions, "create_dry_run") {
		t.Fatalf("draft runner binding should not offer dry run: %+v", matches.Items[0].NextActions)
	}

	if _, err := service.SaveRunnerBindings(pack.ID, []ExperiencePackRunnerBinding{{
		ID:           "binding-published",
		WorkflowID:   "wf-published",
		WorkflowName: "Checkout Published Workflow",
		Status:       "published",
		ReviewStatus: "approved",
		Metadata:     map[string]any{"published": true},
	}}); err != nil {
		t.Fatalf("save published binding: %v", err)
	}
	matches, err = service.Retrieve(ExperiencePackRetrieveRequest{UserText: "checkout"})
	if err != nil {
		t.Fatalf("retrieve with published binding: %v", err)
	}
	if !containsExperiencePackAction(matches.Items[0].NextActions, "create_dry_run") {
		t.Fatalf("published runner binding should offer dry run: %+v", matches.Items[0].NextActions)
	}
	if matches.Items[0].RunnerBinding.WorkflowID != "wf-published" {
		t.Fatalf("runner binding=%+v want wf-published", matches.Items[0].RunnerBinding)
	}
}

func containsExperiencePackAction(actions []string, want string) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}
