# aiops-v2 Agent UI Lightweight Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the lightweight Agent-to-UI protocol registry described in `docs/superpowers/specs/2026-05-16-aiops-v2-agent-ui-lightweight-registry-design.zh.md`, so existing `AgentUIArtifact` values can be schema-validated, previewed, versioned, managed, and rendered through a local trusted UI card registry.

**Architecture:** Keep the existing transport field `agentUiArtifacts` as the runtime contract. Add a Go `UICardService` over the existing store, expose real `/api/v1/ui-cards` and `/api/v1/agent-ui-artifacts` endpoints, then make the frontend registry the single renderer selection mechanism for every normalized artifact, including unsupported and invalid terminal states.

**Tech Stack:** Go HTTP server, existing `internal/store` JSON/Gorm stores, React 19, TypeScript, Vite, Vitest, Playwright, existing shadcn-style UI primitives, existing assistant transport.

---

## Source Spec

- Design: `docs/superpowers/specs/2026-05-16-aiops-v2-agent-ui-lightweight-registry-design.zh.md`
- Existing implementation plan for older Agent-to-UI work: `docs/superpowers/plans/2026-05-13-aiops-v2-agent-to-ui-implementation.md`
- Existing transport model: `internal/appui/transport_state.go`
- Existing frontend normalizer: `web/src/api/agentUiArtifacts.ts`
- Existing renderer dispatcher: `web/src/components/chat/AgentUiArtifactPart.tsx`
- Existing UI card page: `web/src/pages/UICardManagementPage.tsx`
- Existing empty backend resource handler: `internal/server/resource_management.go`

Implementation decision for this plan: registry lookup directly replaces the existing dispatcher. If older design text permits keeping hardcoded rendering as a transition path, this plan supersedes that text.

## Execution Rules

- Work from a clean implementation branch or isolated worktree before starting.
- Keep implementation commits small: one task, one commit.
- Do not revert unrelated existing changes in the repository.
- Use TDD for each code task: failing test, minimal implementation, passing test.
- Do not allow model-generated HTML, scripts, iframes, event handlers, or dynamic React code into renderers.
- Replace legacy artifact rendering with registry lookup. `AgentUiArtifactPart` must not keep a direct type switch, type-specific early returns, or any second rendering path.
- Use `items/stats/total` in `/api/v1/ui-cards` responses; keep a `cards` compatibility alias only if needed by old callers.

## Non-Negotiable Rendering Rule

`lookupAgentUiCardRenderer` is the only renderer selection path for Chat artifacts. Unsupported, disabled, missing-renderer, and invalid-payload cases must be represented as registry-owned terminal renderers. They must not fall back to the old switch dispatcher, dedicated component branches, or any legacy compatibility path.

The existing artifact type `ops_manual_fallback_guide` is a business artifact type name and remains supported as a registered renderer. It is not a renderer fallback mechanism.

## File Map

### Backend

- Modify: `internal/store/store.go`
  - Extend `UICard` with schema, samples, policies, renderer version, and timestamps.
- Modify: `internal/store/gorm_store.go`
  - Ensure extended `UICard` fields round-trip through existing KV storage.
- Test: `internal/store/gorm_store_test.go`
  - Add JSON and Gorm store UI card round-trip coverage.
- Create: `internal/appui/ui_card_service.go`
  - Own built-in definitions, list/get/create/update/delete/status/version, validate, preview, and stats.
- Create: `internal/appui/ui_card_service_test.go`
  - Cover service behavior independent of HTTP.
- Modify: `internal/appui/contracts.go`
  - Add `UICardService` interface and wire default service into `Services`.
- Modify: `internal/server/resources.go`
  - Give `ResourceServer` access to `appui.UICardService`.
- Modify: `internal/server/resource_routes.go`
  - Register `/api/v1/agent-ui-artifacts` routes.
- Modify: `internal/server/resource_management.go`
  - Replace empty `handleUICards` with real handler methods.
- Create: `internal/server/agent_ui_artifacts.go`
  - Implement artifact feed and validation endpoint.
- Test: `internal/server/ui_cards_api_test.go`
  - Cover UI card CRUD, preview, validation, status, and version endpoints.
- Test: `internal/server/agent_ui_artifacts_api_test.go`
  - Cover artifact feed filters and validation behavior.
- Modify: `internal/server/http.go`
  - Register `ResourceServer` with the service from `HTTPServer`.

### Frontend API and Model

- Modify: `web/src/api/uiCards.js`
  - Add `fetchUiCard`, `createUiCard`, `deleteUiCard`, `validateUiCard`, `updateUiCardStatus`, and `createUiCardVersion`.
- Create: `web/src/api/uiCards.test.js`
  - Cover client paths and payload shapes.
- Modify: `web/src/api/agentUiArtifacts.ts`
  - Add schema version metadata, action allowlist normalization, richer dangerous key removal, and version-aware registry status metadata.
- Test: `web/src/api/agentUiArtifacts.test.ts`
  - Extend existing tests for registry-ready normalization.
- Create: `web/src/lib/agentUiCardRegistry.tsx`
  - Provide frontend renderer registry and lookup rules.
- Create: `web/src/lib/agentUiCardRegistry.test.tsx`
  - Cover active, deprecated, disabled, missing renderer, invalid payload, and registry terminal-state lookups.
- Create: `web/src/lib/agentUiCardDefinitions.ts`
  - Define built-in frontend definitions that match backend seeds.

### Frontend Rendering and Pages

- Modify: `web/src/components/chat/AgentUiArtifactPart.tsx`
  - Replace the existing dispatcher with registry lookup as the only renderer selection mechanism.
- Test: `web/src/components/chat/AgentUiArtifactPart.test.tsx`
  - Add registry lookup and terminal-state cases without removing existing assertions.
- Create: `web/src/components/chat/UnsupportedArtifactCard.tsx`
  - Registry-owned terminal renderer for unregistered or disabled artifacts.
- Create: `web/src/components/chat/InvalidArtifactCard.tsx`
  - Registry-owned terminal renderer for schema validation failures.
- Create: `web/src/pages/AgentUICenterPage.tsx`
  - Cross-page artifact feed and detail drawer.
- Create: `web/src/pages/AgentUICenterPage.test.tsx`
  - Cover filters, detail drawer, and trace links.
- Modify: `web/src/pages/UICardManagementPage.tsx`
  - Upgrade from simple debug page to registry list/detail/preview/version page.
- Test: `web/src/pages/settingsPages.test.tsx` or create `web/src/pages/UICardManagementPage.test.tsx`
  - Cover registry management UI.
- Modify: `web/src/router.tsx`
  - Add `/agent-ui` route.
- Modify: `web/src/app/navigation.ts`
  - Add Agent UI Center navigation item.
- Modify: `web/src/lib/zhLabels.ts`
  - Add Chinese route/card type labels.
- Test: `web/src/router.test.tsx`
  - Cover route inventory.

### End-to-End and Docs

- Create: `web/tests/agent-ui-registry.spec.js`
  - Verify `/ui-cards`, preview, Chat registry terminal states, and `/agent-ui` feed flows.
- Modify: `docs/superpowers/specs/2026-05-16-aiops-v2-agent-ui-lightweight-registry-design.zh.md`
  - Update only if implementation reveals a design correction.

---

## Task 1: Extend UI Card Store Model

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/gorm_store.go`
- Test: `internal/store/gorm_store_test.go`

- [x] **Step 1: Write a failing JSON store round-trip test**

Add a test in `internal/store/gorm_store_test.go` named `TestJSONFileStore_UICardsExtendedFieldsRoundTrip`. The test should create a temp JSON store, save one UI card with schema, policies, samples, and version fields, flush, reopen the store, and assert every nested field is preserved.

Use this card shape in the test:

```go
card := store.UICard{
	ID:              "metric-insight",
	Name:            "Metric Insight",
	Kind:            "metric",
	Renderer:        "MetricInsightCard",
	RendererVersion: "MetricInsightCard@1",
	SchemaVersion:   "json-schema-draft-2020-12",
	PayloadSchema: map[string]any{
		"type":     "object",
		"required": []any{"metrics"},
		"properties": map[string]any{
			"metrics": map[string]any{"type": "array"},
		},
	},
	ActionPolicy: store.UICardActionPolicy{
		AllowedIntents:      []string{"open_case", "open_prompt_trace"},
		MutationAllowed:     false,
		DefaultApprovalMode: "none",
	},
	PlacementDefaults: []string{"inline_final", "drawer"},
	DisplayPolicy: store.UICardDisplayPolicy{
		MaxInlineHeightPx: 320,
		DefaultDensity:    "compact",
		DetailMode:        "drawer",
		EmptyState:        "暂无指标数据",
	},
	RedactionPolicy: store.UICardRedactionPolicy{
		RestrictedFields:          []string{"payload.secret"},
		SummaryOnlyWhenRestricted: true,
	},
	SamplePayloads: []store.UICardSample{
		{
			ID:   "warning-latency",
			Name: "延迟升高",
			Artifact: map[string]any{
				"id":       "artifact-sample-1",
				"type":     "metric_insight",
				"titleZh":  "支付延迟升高",
				"status":   "warning",
				"payload":  map[string]any{"metrics": []any{map[string]any{"name": "p95", "value": 2800}}},
				"metadata": map[string]any{"schemaVersion": "aiops.agent_ui.v1"},
			},
		},
	},
	Status:  "active",
	BuiltIn: true,
	Version: 1,
}
```

- [x] **Step 2: Run the failing store test**

Run:

```bash
go test ./internal/store -run TestJSONFileStore_UICardsExtendedFieldsRoundTrip -count=1
```

Expected: fail because `store.UICard` does not yet define the extended fields and nested policy types.

- [x] **Step 3: Extend the store types**

In `internal/store/store.go`, add focused nested types near `UICard`, then extend `UICard` by appending fields. Use `map[string]any` for schemas so JSON and Gorm KV storage remain simple.

```go
type UICardActionPolicy struct {
	AllowedIntents      []string `json:"allowedIntents,omitempty"`
	MutationAllowed     bool     `json:"mutationAllowed"`
	DefaultApprovalMode string   `json:"defaultApprovalMode,omitempty"`
}

type UICardDisplayPolicy struct {
	MaxInlineHeightPx int    `json:"maxInlineHeightPx,omitempty"`
	DefaultDensity    string `json:"defaultDensity,omitempty"`
	DetailMode        string `json:"detailMode,omitempty"`
	EmptyState        string `json:"emptyState,omitempty"`
}

type UICardRedactionPolicy struct {
	RestrictedFields          []string `json:"restrictedFields,omitempty"`
	SummaryOnlyWhenRestricted bool     `json:"summaryOnlyWhenRestricted"`
}

type UICardSample struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description,omitempty"`
	Artifact         map[string]any `json:"artifact"`
	ExpectedStatus   string         `json:"expectedStatus,omitempty"`
	ExpectedWarnings []string       `json:"expectedWarnings,omitempty"`
}
```

Append these fields to `UICard`:

```go
RendererVersion string                 `json:"rendererVersion,omitempty"`
SchemaVersion   string                 `json:"schemaVersion,omitempty"`
PayloadSchema   map[string]any         `json:"payloadSchema,omitempty"`
MetadataSchema  map[string]any         `json:"metadataSchema,omitempty"`
ActionPolicy    UICardActionPolicy     `json:"actionPolicy,omitempty"`
DisplayPolicy   UICardDisplayPolicy    `json:"displayPolicy,omitempty"`
RedactionPolicy UICardRedactionPolicy  `json:"redactionPolicy,omitempty"`
SamplePayloads  []UICardSample         `json:"samplePayloads,omitempty"`
```

- [x] **Step 4: Run JSON and Gorm store tests**

Run:

```bash
go test ./internal/store -run 'UICards|GormStore|JSONFileStore' -count=1
```

Expected: pass.

- [x] **Step 5: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add internal/store/store.go internal/store/gorm_store.go internal/store/gorm_store_test.go
git commit -m "feat: extend ui card store model"
```

---

## Task 2: Add UI Card Domain Service

**Files:**
- Create: `internal/appui/ui_card_service.go`
- Create: `internal/appui/ui_card_service_test.go`
- Modify: `internal/appui/contracts.go`

- [x] **Step 1: Write service tests for list, seed, preview, and validation**

Create `internal/appui/ui_card_service_test.go` with tests named:

- `TestUICardService_SeedsBuiltInDefinitionsWhenStoreIsEmpty`
- `TestUICardService_ListReturnsStatsAndSortedItems`
- `TestUICardService_ValidateRejectsDangerousPayloadKeys`
- `TestUICardService_PreviewReturnsNormalizedDefinitionMetadata`
- `TestUICardService_UpdateStatusRejectsUnknownStatus`

The dangerous-key test must submit an artifact containing:

```go
artifact := map[string]any{
	"id":      "artifact-dangerous",
	"type":    "metric_insight",
	"titleZh": "危险字段样例",
	"payload": map[string]any{
		"html":                    "<img src=x onerror=alert(1)>",
		"script":                  "alert(1)",
		"dangerouslySetInnerHTML": map[string]any{"__html": "bad"},
		"metrics":                 []any{},
	},
}
```

Expected validation result:

```text
valid=false
errors contains payload.html
errors contains payload.script
errors contains payload.dangerouslySetInnerHTML
```

- [x] **Step 2: Run the failing service tests**

Run:

```bash
go test ./internal/appui -run UICardService -count=1
```

Expected: fail because the service does not exist.

- [x] **Step 3: Define service contracts**

In `internal/appui/contracts.go`, add:

```go
type UICardService interface {
	ListUICards(ctx context.Context) (UICardListResult, error)
	GetUICard(ctx context.Context, id string) (store.UICard, bool, error)
	CreateUICard(ctx context.Context, card store.UICard) (store.UICard, error)
	UpdateUICard(ctx context.Context, id string, card store.UICard) (store.UICard, error)
	DeleteUICard(ctx context.Context, id string) error
	UpdateUICardStatus(ctx context.Context, id string, status string) (store.UICard, error)
	CreateUICardVersion(ctx context.Context, id string) (store.UICard, error)
	ValidateUICardArtifact(ctx context.Context, cardID string, artifact map[string]any) (UICardValidationResult, error)
	PreviewUICard(ctx context.Context, cardID string, artifact map[string]any, context map[string]any) (UICardPreviewResult, error)
}
```

Also add `UICardService() UICardService` to `HTTPServices` and `Services`, wire a default service in `NewServices`, and add a `WithUICardService` option if test setup needs injection.

- [x] **Step 4: Implement minimal service behavior**

Create `internal/appui/ui_card_service.go` with:

- `UICardListResult` containing `Items []store.UICard`, `Stats map[string]int`, and `Total int`.
- `UICardValidationResult` containing `Valid bool`, `Errors []UICardValidationError`, `Warnings []string`, and `RedactedPaths []string`.
- `UICardPreviewResult` containing `Valid bool`, `DefinitionID string`, `DefinitionVersion int`, `Renderer string`, `NormalizedArtifact map[string]any`, `Warnings []string`, and `RedactedPaths []string`.
- Built-in definitions for current artifact types: `coroot_chart`, `trace_summary`, `topology_slice`, `workflow_result`, `verification_result`, `experience_match`, `ops_manual_match`, `ops_manual_search_result`, `ops_manual_preflight_result`, `ops_manual_fallback_guide`, `runner_workflow_generation`.
- Dangerous key traversal for `html`, `script`, `iframe`, `innerHTML`, `outerHTML`, `dangerouslySetInnerHTML`, `onClick`, `onLoad`, and `styleText`.

- [x] **Step 5: Run service tests**

Run:

```bash
go test ./internal/appui -run UICardService -count=1
```

Expected: pass.

- [x] **Step 6: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add internal/appui/contracts.go internal/appui/ui_card_service.go internal/appui/ui_card_service_test.go
git commit -m "feat: add ui card domain service"
```

---

## Task 3: Replace Empty UI Card HTTP Handler

**Files:**
- Modify: `internal/server/resources.go`
- Modify: `internal/server/resource_management.go`
- Modify: `internal/server/http.go`
- Test: `internal/server/ui_cards_api_test.go`
- Test: `internal/server/resources_http_test.go`

- [x] **Step 1: Write HTTP API tests**

Create `internal/server/ui_cards_api_test.go` with tests:

- `TestUICardsAPI_ListReturnsItemsStatsAndTotal`
- `TestUICardsAPI_GetReturnsCardByID`
- `TestUICardsAPI_PutUpdatesEditableDefinitionFields`
- `TestUICardsAPI_StatusUpdatesCardLifecycle`
- `TestUICardsAPI_PreviewValidatesArtifactPayload`
- `TestUICardsAPI_DeleteRejectsBuiltInCards`

The list test must assert response keys:

```go
if _, ok := payload["items"]; !ok {
	t.Fatalf("response missing items: %#v", payload)
}
if _, ok := payload["stats"]; !ok {
	t.Fatalf("response missing stats: %#v", payload)
}
if _, ok := payload["total"]; !ok {
	t.Fatalf("response missing total: %#v", payload)
}
```

- [x] **Step 2: Run failing HTTP tests**

Run:

```bash
go test ./internal/server -run UICardsAPI -count=1
```

Expected: fail because `handleUICards` still returns `{"cards":[]}` and does not route subresources.

- [x] **Step 3: Wire ResourceServer to UICardService**

Modify `internal/server/resources.go` so `ResourceServer` stores an `appui.UICardService`.

```go
type ResourceServer struct {
	mux     *http.ServeMux
	coroot  corootProxyConfig
	uiCards appui.UICardService
}
```

Add a constructor:

```go
func NewResourceServerWithUICards(uiCards appui.UICardService) *ResourceServer {
	rs := &ResourceServer{
		mux:     http.NewServeMux(),
		coroot:  corootProxyConfigFromEnv(),
		uiCards: uiCards,
	}
	rs.registerRoutes()
	return rs
}
```

Keep `NewResourceServer()` as a compatibility wrapper that creates a default service.

- [x] **Step 4: Register resource routes from HTTPServer with the app service**

In `internal/server/http.go`, replace:

```go
NewResourceServer().RegisterOnMux(s.mux)
```

with:

```go
NewResourceServerWithUICards(s.ui.UICardService()).RegisterOnMux(s.mux)
```

- [x] **Step 5: Implement UI card route parsing**

In `internal/server/resource_management.go`, route:

```text
GET    /api/v1/ui-cards
POST   /api/v1/ui-cards
GET    /api/v1/ui-cards/{id}
PUT    /api/v1/ui-cards/{id}
DELETE /api/v1/ui-cards/{id}
POST   /api/v1/ui-cards/{id}/preview
POST   /api/v1/ui-cards/{id}/validate
POST   /api/v1/ui-cards/{id}/versions
PUT    /api/v1/ui-cards/{id}/status
```

Use small helper functions in the same file:

- `uiCardPathParts(path string) []string`
- `decodeResourceJSON(r *http.Request, dst any) bool`
- `writeResourceError(w http.ResponseWriter, status int, message string)`

- [x] **Step 6: Run server tests**

Run:

```bash
go test ./internal/server -run 'UICardsAPI|RegistersResourceRoutes' -count=1
```

Expected: pass.

- [x] **Step 7: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add internal/server/resources.go internal/server/resource_management.go internal/server/http.go internal/server/ui_cards_api_test.go internal/server/resources_http_test.go
git commit -m "feat: expose ui card registry api"
```

---

## Task 4: Add Agent UI Artifact Feed API

**Files:**
- Create: `internal/appui/agent_ui_artifact_service.go`
- Create: `internal/appui/agent_ui_artifact_service_test.go`
- Create: `internal/server/agent_ui_artifacts.go`
- Modify: `internal/server/resource_routes.go`
- Test: `internal/server/agent_ui_artifacts_api_test.go`

- [x] **Step 1: Write service tests for artifact feed aggregation**

Create tests that seed a small in-memory set of artifacts and assert:

- filtering by `source=coroot`
- filtering by `type=coroot_chart`
- filtering by `caseId=case-debug-1`
- `limit=1` returns one item and a non-empty next cursor when more items exist
- `validate` rejects unknown dangerous keys using the same dangerous key list as UI cards

- [x] **Step 2: Run failing service tests**

Run:

```bash
go test ./internal/appui -run AgentUIArtifact -count=1
```

Expected: fail because the service does not exist.

- [x] **Step 3: Implement an initial artifact feed service**

Create `internal/appui/agent_ui_artifact_service.go` with:

```go
type AgentUIArtifactQuery struct {
	Source        string
	Type          string
	Status        string
	CaseID        string
	PromptTraceID string
	SubjectKind   string
	SubjectID     string
	Limit         int
	Cursor        string
}

type AgentUIArtifactListResult struct {
	Items      []AiopsTransportAgentUIArtifact `json:"items"`
	NextCursor string                          `json:"nextCursor,omitempty"`
	Total      int                             `json:"total"`
}
```

First implementation can read from an injected in-memory provider and expose a clean interface. It should be designed so later work can plug in session snapshots, cases, prompt traces, runner records, and Coroot evidence without changing HTTP response shape.

- [x] **Step 4: Write HTTP API tests**

Create `internal/server/agent_ui_artifacts_api_test.go` with tests:

- `TestAgentUIArtifactsAPI_ListFiltersBySourceAndType`
- `TestAgentUIArtifactsAPI_GetMissingReturnsNotFound`
- `TestAgentUIArtifactsAPI_ValidateRejectsDangerousPayload`

- [x] **Step 5: Register and implement endpoints**

In `internal/server/resource_routes.go`, register:

```go
mux.HandleFunc("/api/v1/agent-ui-artifacts", rs.handleAgentUIArtifacts)
mux.HandleFunc("/api/v1/agent-ui-artifacts/", rs.handleAgentUIArtifacts)
```

Create `internal/server/agent_ui_artifacts.go` and implement:

```text
GET  /api/v1/agent-ui-artifacts
GET  /api/v1/agent-ui-artifacts/{id}
POST /api/v1/agent-ui-artifacts/validate
```

- [x] **Step 6: Run backend artifact API tests**

Run:

```bash
go test ./internal/appui ./internal/server -run 'AgentUIArtifact|AgentUIArtifactsAPI' -count=1
```

Expected: pass.

- [x] **Step 7: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add internal/appui/agent_ui_artifact_service.go internal/appui/agent_ui_artifact_service_test.go internal/server/agent_ui_artifacts.go internal/server/agent_ui_artifacts_api_test.go internal/server/resource_routes.go
git commit -m "feat: add agent ui artifact feed api"
```

---

## Task 5: Upgrade Frontend UI Card API Client

**Files:**
- Modify: `web/src/api/uiCards.js`
- Create: `web/src/api/uiCards.test.js`

- [x] **Step 1: Write client tests**

Create `web/src/api/uiCards.test.js` using `createHttpClient` with a fake `fetchImpl`. Assert exact paths:

```text
GET    /api/v1/ui-cards
GET    /api/v1/ui-cards/metric-insight
POST   /api/v1/ui-cards
PUT    /api/v1/ui-cards/metric-insight
DELETE /api/v1/ui-cards/metric-insight
POST   /api/v1/ui-cards/metric-insight/preview
POST   /api/v1/ui-cards/metric-insight/validate
POST   /api/v1/ui-cards/metric-insight/versions
PUT    /api/v1/ui-cards/metric-insight/status
```

- [x] **Step 2: Run failing client tests**

Run:

```bash
cd web
npm test -- --run src/api/uiCards.test.js
```

Expected: fail because the new client functions do not exist.

- [x] **Step 3: Implement client functions**

Update `web/src/api/uiCards.js` with:

```js
export function fetchUiCards() {
  return httpClient.get("/api/v1/ui-cards");
}

export function fetchUiCard(id) {
  return httpClient.get(`/api/v1/ui-cards/${encodeURIComponent(id)}`);
}

export function createUiCard(payload) {
  return httpClient.post("/api/v1/ui-cards", payload);
}

export function updateUiCard(id, payload) {
  return httpClient.put(`/api/v1/ui-cards/${encodeURIComponent(id)}`, payload);
}

export function deleteUiCard(id) {
  return httpClient.delete(`/api/v1/ui-cards/${encodeURIComponent(id)}`);
}

export function previewUiCard(id, payload) {
  return httpClient.post(`/api/v1/ui-cards/${encodeURIComponent(id)}/preview`, payload);
}

export function validateUiCard(id, payload) {
  return httpClient.post(`/api/v1/ui-cards/${encodeURIComponent(id)}/validate`, payload);
}

export function createUiCardVersion(id, payload = {}) {
  return httpClient.post(`/api/v1/ui-cards/${encodeURIComponent(id)}/versions`, payload);
}

export function updateUiCardStatus(id, status) {
  return httpClient.put(`/api/v1/ui-cards/${encodeURIComponent(id)}/status`, { status });
}
```

- [x] **Step 4: Run client tests**

Run:

```bash
cd web
npm test -- --run src/api/uiCards.test.js
```

Expected: pass.

- [x] **Step 5: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/api/uiCards.js web/src/api/uiCards.test.js
git commit -m "feat: expand ui card api client"
```

---

## Task 6: Harden Agent UI Artifact Normalizer

**Files:**
- Modify: `web/src/api/agentUiArtifacts.ts`
- Modify: `web/src/api/agentUiArtifacts.test.ts`

- [x] **Step 1: Add failing tests for action allowlist and dangerous keys**

Extend `web/src/api/agentUiArtifacts.test.ts` with tests that assert:

- `iframe`, `outerHTML`, `onClick`, `onLoad`, and `styleText` are removed recursively.
- known intents remain enabled: `open_case`, `open_evidence`, `open_prompt_trace`, `open_coroot`, `open_workflow_run`, `request_approval`, `start_dry_run`.
- unknown mutation action is returned with `disabled: true` and a `disabledReason`.
- `metadata.schemaVersion`, `metadata.cardVersion`, and `metadata.renderer` are preserved.

- [x] **Step 2: Run failing normalizer tests**

Run:

```bash
cd web
npm test -- --run src/api/agentUiArtifacts.test.ts
```

Expected: fail for new dangerous keys and action policy assertions.

- [x] **Step 3: Extend normalizer constants**

In `web/src/api/agentUiArtifacts.ts`, expand dangerous keys:

```ts
const DANGEROUS_KEYS = new Set([
  "html",
  "script",
  "iframe",
  "innerHTML",
  "outerHTML",
  "dangerouslySetInnerHTML",
  "onClick",
  "onLoad",
  "styleText",
]);
```

Add action intent allowlist:

```ts
const ALLOWED_ACTION_INTENTS = new Set([
  "open_case",
  "open_evidence",
  "open_prompt_trace",
  "open_coroot",
  "open_workflow_run",
  "open_runner",
  "open_ops_manual",
  "request_approval",
  "start_dry_run",
  "attach_to_case",
]);
```

- [x] **Step 4: Normalize unknown actions safely**

Update action normalization so unknown intents are not dropped silently. Return:

```ts
{
  ...action,
  disabled: true,
  disabledReason: `不支持的操作 intent: ${intent}`,
}
```

Do not execute or route unknown actions.

- [x] **Step 5: Run normalizer tests**

Run:

```bash
cd web
npm test -- --run src/api/agentUiArtifacts.test.ts
```

Expected: pass.

- [x] **Step 6: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/api/agentUiArtifacts.ts web/src/api/agentUiArtifacts.test.ts
git commit -m "feat: harden agent ui artifact normalization"
```

---

## Task 7: Add Frontend Card Registry Runtime

**Files:**
- Create: `web/src/lib/agentUiCardDefinitions.ts`
- Create: `web/src/lib/agentUiCardRegistry.tsx`
- Create: `web/src/lib/agentUiCardRegistry.test.tsx`

- [x] **Step 1: Write registry lookup tests**

Create tests covering:

- active definition returns renderer for `coroot_chart`
- deprecated definition returns renderer and warning
- disabled definition returns `UnsupportedArtifactCard`
- missing renderer returns `UnsupportedArtifactCard`
- invalid payload result returns `InvalidArtifactCard`
- artifact without `metadata.schemaVersion` still renders through the active registry definition

- [x] **Step 2: Run failing registry tests**

Run:

```bash
cd web
npm test -- --run src/lib/agentUiCardRegistry.test.tsx
```

Expected: fail because registry files do not exist.

- [x] **Step 3: Define built-in frontend definitions**

Create `web/src/lib/agentUiCardDefinitions.ts` with definitions for existing supported types. Include:

```ts
export const BUILTIN_AGENT_UI_CARD_DEFINITIONS = [
  { id: "coroot-chart", type: "coroot_chart", renderer: "CorootChartArtifact", status: "active", version: 1 },
  { id: "trace-summary", type: "trace_summary", renderer: "TraceSummaryArtifact", status: "active", version: 1 },
  { id: "topology-slice", type: "topology_slice", renderer: "TopologySliceArtifact", status: "active", version: 1 },
  { id: "workflow-result", type: "workflow_result", renderer: "WorkflowResultArtifact", status: "active", version: 1 },
  { id: "verification-result", type: "verification_result", renderer: "VerificationResultArtifact", status: "active", version: 1 },
  { id: "experience-match", type: "experience_match", renderer: "ExperienceMatchArtifact", status: "active", version: 1 },
  { id: "ops-manual-match", type: "ops_manual_match", renderer: "OpsManualMatchArtifact", status: "active", version: 1 },
  { id: "ops-manual-search-result", type: "ops_manual_search_result", renderer: "OpsManualSearchResultArtifact", status: "active", version: 1 },
  { id: "ops-manual-preflight-result", type: "ops_manual_preflight_result", renderer: "OpsManualPreflightResultArtifact", status: "active", version: 1 },
  { id: "ops-manual-fallback-guide", type: "ops_manual_fallback_guide", renderer: "OpsManualFallbackGuideArtifact", status: "active", version: 1 },
  { id: "runner-workflow-generation", type: "runner_workflow_generation", renderer: "RunnerWorkflowGenerationArtifact", status: "active", version: 1 },
];
```

- [x] **Step 4: Implement registry lookup**

Create `web/src/lib/agentUiCardRegistry.tsx` with:

- `createAgentUiCardRegistry(definitions, renderers)`
- `lookupAgentUiCardRenderer(artifact, registry)`
- `AgentUiCardLookupResult`

The lookup result must contain:

```ts
{
  kind: "ready" | "unsupported" | "invalid";
  definition?: AgentUiCardDefinition;
  Renderer: React.ComponentType<{ artifact: AiopsTransportAgentUiArtifact }>;
  warnings: string[];
}
```

- [x] **Step 5: Run registry tests**

Run:

```bash
cd web
npm test -- --run src/lib/agentUiCardRegistry.test.tsx
```

Expected: pass.

- [x] **Step 6: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/lib/agentUiCardDefinitions.ts web/src/lib/agentUiCardRegistry.tsx web/src/lib/agentUiCardRegistry.test.tsx
git commit -m "feat: add agent ui card registry runtime"
```

---

## Task 8: Route Chat Rendering Through Registry

**Files:**
- Modify: `web/src/components/chat/AgentUiArtifactPart.tsx`
- Create: `web/src/components/chat/UnsupportedArtifactCard.tsx`
- Create: `web/src/components/chat/InvalidArtifactCard.tsx`
- Modify: `web/src/components/chat/AgentUiArtifactPart.test.tsx`

- [x] **Step 1: Add failing dispatcher tests**

Add tests to `AgentUiArtifactPart.test.tsx`:

- disabled registry definition renders `UnsupportedArtifactCard` through registry lookup
- missing renderer renders `UnsupportedArtifactCard` through registry lookup
- invalid payload renders `InvalidArtifactCard` through registry lookup
- existing `coroot_chart` still renders `CorootChartArtifact`
- existing `ops_manual_preflight_result` still renders the registered ops manual preflight renderer

- [x] **Step 2: Run failing component tests**

Run:

```bash
cd web
npm test -- --run src/components/chat/AgentUiArtifactPart.test.tsx
```

Expected: fail because the dispatcher does not use registry lookup.

- [x] **Step 3: Implement registry terminal renderers**

Create `UnsupportedArtifactCard.tsx` and `InvalidArtifactCard.tsx`. Both must:

- render title, summary, type, source, and trace links when available
- avoid rendering `payload` directly
- avoid `dangerouslySetInnerHTML`
- show a short Chinese reason
- be invoked only by `lookupAgentUiCardRenderer`, not by ad hoc branches in `AgentUiArtifactPart.tsx`

- [x] **Step 4: Update dispatcher**

In `AgentUiArtifactPart.tsx`, delete the direct type switch and type-specific early returns. Replace them with:

```tsx
const lookup = lookupAgentUiCardRenderer(artifact, defaultAgentUiCardRegistry);
const Renderer = lookup.Renderer;
return <Renderer artifact={artifact} />;
```

All current dedicated components, including ops manual components, must be reachable through `defaultAgentUiCardRegistry`. If a renderer needs event-dispatch behavior, register that renderer in the registry instead of adding a special-case branch.

- [x] **Step 5: Run component tests**

Run:

```bash
cd web
npm test -- --run src/components/chat/AgentUiArtifactPart.test.tsx src/lib/agentUiCardRegistry.test.tsx
```

Expected: pass.

- [x] **Step 6: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/components/chat/AgentUiArtifactPart.tsx web/src/components/chat/AgentUiArtifactPart.test.tsx web/src/components/chat/UnsupportedArtifactCard.tsx web/src/components/chat/InvalidArtifactCard.tsx
git commit -m "feat: render agent ui artifacts through registry"
```

---

## Task 9: Upgrade UI Card Management Page

**Files:**
- Modify: `web/src/pages/UICardManagementPage.tsx`
- Create: `web/src/pages/UICardManagementPage.test.tsx`
- Modify: `web/src/pages/settingsPages.test.tsx` if route-level coverage already belongs there

- [x] **Step 1: Write page tests**

Create tests that mock `/api/v1/ui-cards` and assert:

- stats row shows total, active, draft, deprecated, disabled, built-in
- registry list shows name, type, renderer, status, version
- detail panel shows schema, action policy, redaction policy, placement defaults
- preview tab posts to `/api/v1/ui-cards/{id}/preview`
- invalid JSON in preview input shows a local validation message and does not call the API
- status action calls `/api/v1/ui-cards/{id}/status`

- [x] **Step 2: Run failing page tests**

Run:

```bash
cd web
npm test -- --run src/pages/UICardManagementPage.test.tsx
```

Expected: fail because the current page only has overview/list/debugger and minimal editing.

- [x] **Step 3: Implement registry page tabs**

Update page tabs to:

```text
Overview
Registry
Detail
Preview
Versions
```

Use existing UI primitives from `web/src/components/ui/*` and existing page shells. Keep the layout dense and operational:

- left list for definitions
- right detail or preview panel
- no marketing-style hero blocks
- no nested cards inside cards

- [x] **Step 4: Implement preview workflow**

Preview tab behavior:

- selected card defaults to first item
- sample selector uses `samplePayloads`
- JSON editor starts with selected sample artifact
- permission context selector supports `normal`, `redacted`, `restricted`
- submit calls `previewUiCard(id, { artifact, context })`
- render API result and local registry preview side by side when possible

- [x] **Step 5: Run page tests**

Run:

```bash
cd web
npm test -- --run src/pages/UICardManagementPage.test.tsx
```

Expected: pass.

- [x] **Step 6: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/pages/UICardManagementPage.tsx web/src/pages/UICardManagementPage.test.tsx web/src/pages/settingsPages.test.tsx
git commit -m "feat: upgrade ui card management page"
```

---

## Task 10: Add Agent UI Center Page

**Files:**
- Create: `web/src/api/agentUiArtifactsClient.ts`
- Create: `web/src/pages/AgentUICenterPage.tsx`
- Create: `web/src/pages/AgentUICenterPage.test.tsx`
- Modify: `web/src/router.tsx`
- Modify: `web/src/app/navigation.ts`
- Modify: `web/src/lib/zhLabels.ts`
- Modify: `web/src/router.test.tsx`

- [x] **Step 1: Write API client and page tests**

Tests must assert:

- `fetchAgentUiArtifacts({ source: "coroot", type: "coroot_chart" })` calls `/api/v1/agent-ui-artifacts?source=coroot&type=coroot_chart`
- page renders artifact feed rows
- filters update request query
- selecting an artifact opens detail drawer
- detail drawer includes normalized JSON, metadata, actions, and trace links
- route `/agent-ui` renders the page

- [x] **Step 2: Run failing tests**

Run:

```bash
cd web
npm test -- --run src/pages/AgentUICenterPage.test.tsx src/router.test.tsx
```

Expected: fail because the page and route do not exist.

- [x] **Step 3: Implement API client**

Create `web/src/api/agentUiArtifactsClient.ts` with:

```ts
export function fetchAgentUiArtifacts(filters: Record<string, string> = {}) {
  const params = new URLSearchParams(Object.entries(filters).filter(([, value]) => value));
  const suffix = params.toString() ? `?${params.toString()}` : "";
  return httpClient.get(`/api/v1/agent-ui-artifacts${suffix}`);
}

export function fetchAgentUiArtifact(id: string) {
  return httpClient.get(`/api/v1/agent-ui-artifacts/${encodeURIComponent(id)}`);
}

export function validateAgentUiArtifact(payload: Record<string, unknown>) {
  return httpClient.post("/api/v1/agent-ui-artifacts/validate", payload);
}
```

- [x] **Step 4: Implement Agent UI Center**

The page should include:

- metric strip: total, warning/error, restricted, unsupported
- filters: source, type, status, caseId, promptTraceId
- feed table/list: title, type, status, source, subject, updatedAt
- detail drawer: rendered card preview, normalized JSON, metadata, actions, raw payload
- quick links: Case, Evidence, Prompt Trace, Workflow Run, Coroot

- [x] **Step 5: Wire route and navigation**

Add route `/agent-ui` to:

- `web/src/router.tsx`
- `web/src/app/navigation.ts`
- `web/src/lib/zhLabels.ts`

Navigation title: `Agent UI`
Description: `卡片产物与渲染追踪`

- [x] **Step 6: Run tests**

Run:

```bash
cd web
npm test -- --run src/pages/AgentUICenterPage.test.tsx src/router.test.tsx src/lib/zhLabels.test.ts
```

Expected: pass.

- [x] **Step 7: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/src/api/agentUiArtifactsClient.ts web/src/pages/AgentUICenterPage.tsx web/src/pages/AgentUICenterPage.test.tsx web/src/router.tsx web/src/app/navigation.ts web/src/lib/zhLabels.ts web/src/router.test.tsx web/src/lib/zhLabels.test.ts
git commit -m "feat: add agent ui center page"
```

---

## Task 11: Connect Backend and Frontend Contracts

**Files:**
- Modify: `internal/appui/ui_card_service.go`
- Modify: `web/src/lib/agentUiCardDefinitions.ts`
- Modify: `web/src/pages/UICardManagementPage.tsx`
- Test: `internal/appui/ui_card_service_test.go`
- Test: `web/src/lib/agentUiCardRegistry.test.tsx`

- [x] **Step 1: Add a parity test for built-in card definitions**

In backend tests, assert built-in definitions include exactly the current supported artifact type set:

```text
coroot_chart
trace_summary
topology_slice
workflow_result
verification_result
experience_match
ops_manual_match
ops_manual_search_result
ops_manual_preflight_result
ops_manual_fallback_guide
runner_workflow_generation
```

In frontend tests, assert the same set exists in `BUILTIN_AGENT_UI_CARD_DEFINITIONS`.

- [x] **Step 2: Run parity tests**

Run:

```bash
go test ./internal/appui -run UICardService -count=1
cd web
npm test -- --run src/lib/agentUiCardRegistry.test.tsx
```

Expected: pass after backend and frontend definitions align.

- [x] **Step 3: Add a contract comment**

Add a short comment above backend seed definitions and frontend definitions:

```text
Keep this built-in type set aligned with web/src/lib/agentUiCardDefinitions.ts and internal/appui/ui_card_service.go.
```

- [x] **Step 4: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add internal/appui/ui_card_service.go internal/appui/ui_card_service_test.go web/src/lib/agentUiCardDefinitions.ts web/src/lib/agentUiCardRegistry.test.tsx
git commit -m "test: align built in agent ui card definitions"
```

---

## Task 12: Add End-to-End Coverage

**Files:**
- Create: `web/tests/agent-ui-registry.spec.js`
- Modify: `web/tests/helpers/uiFixtureHarness.js` if the harness needs a card registry fixture
- Modify: `web/src/lib/uiFixturePresets.js` if fixture presets need card samples

- [x] **Step 1: Write Playwright test for UI Cards page**

The test should:

- open `/ui-cards`
- wait for registry stats
- select `coroot_chart`
- open Preview tab
- run preview with the built-in sample
- assert rendered preview contains the card title and no raw JSON dump as primary content

- [x] **Step 2: Write Playwright test for unsupported artifact registry behavior**

The test should use a fixture with an artifact:

```json
{
  "id": "artifact-unknown-widget",
  "type": "shell_widget",
  "titleZh": "未知卡片",
  "summaryZh": "后端返回了未注册 UI artifact。",
  "payload": {
    "script": "alert(1)"
  }
}
```

Expected UI:

```text
暂不支持的卡片类型
shell_widget
```

Expected DOM:

```text
No script tag created from payload
No alert text from payload
```

- [x] **Step 3: Write Playwright test for Agent UI Center**

The test should:

- open `/agent-ui`
- filter `source=coroot`
- select an artifact
- assert detail drawer opens
- assert Prompt Trace link exists when fixture contains `promptTraceId`

- [x] **Step 4: Run Playwright tests**

Run:

```bash
cd web
npm run test:ui -- agent-ui-registry.spec.js --project=chromium
```

Expected: pass.

- [x] **Step 5: Commit skipped because the workspace was already dirty and this turn should not mix unrelated changes into a commit**

```bash
git add web/tests/agent-ui-registry.spec.js web/tests/helpers/uiFixtureHarness.js web/src/lib/uiFixturePresets.js
git commit -m "test: cover agent ui registry flows"
```

---

## Task 13: Full Verification Pass

**Files:**
- No planned source changes unless verification exposes a failure.

- [x] **Step 1: Run focused Go tests**

Run:

```bash
go test ./internal/store ./internal/appui ./internal/server -run 'UICard|AgentUIArtifact|ResourceRoutes' -count=1
```

Expected: pass.

Completed:

```bash
go test ./internal/store ./internal/appui ./internal/server -run 'UICard|AgentUIArtifact|ResourceRoutes' -count=1
```

Result: pass.

- [x] **Step 2: Run focused frontend tests**

Run:

```bash
cd web
npm test -- --run \
  src/api/uiCards.test.js \
  src/api/agentUiArtifacts.test.ts \
  src/lib/agentUiCardRegistry.test.tsx \
  src/components/chat/AgentUiArtifactPart.test.tsx \
  src/pages/UICardManagementPage.test.tsx \
  src/pages/AgentUICenterPage.test.tsx \
  src/router.test.tsx
```

Expected: pass.

Completed:

```bash
cd web
npm test -- --run \
  src/api/uiCards.test.js \
  src/api/agentUiArtifacts.test.ts \
  src/lib/agentUiCardRegistry.test.tsx \
  src/components/chat/AgentUiArtifactPart.test.tsx \
  src/pages/UICardManagementPage.test.tsx \
  src/pages/AgentUICenterPage.test.tsx \
  src/router.test.tsx
```

Result: pass. jsdom emitted the known non-fatal `HTMLCanvasElement.getContext` warning.

- [x] **Step 3: Run build**

Run:

```bash
cd web
npm run build
```

Expected: build succeeds.

Completed:

```bash
cd web
npm run build
```

Result: pass. Vite emitted the existing chunk-size warning only.

- [x] **Step 4: Run UI smoke test**

Run:

```bash
cd web
npm run test:ui -- agent-ui-registry.spec.js --project=chromium
```

Expected: pass.

Completed:

```bash
cd web
npm run test:ui -- e2e/agent-ui-registry.spec.js --project=chromium
```

Result: pass.

- [x] **Step 5: Inspect git diff**

Run:

```bash
git diff --stat
git status --short
```

Expected:

- only files from this plan are changed
- no unrelated generated files are staged
- no secrets or large trace artifacts are added

Completed:

```bash
git diff --stat
git status --short
git diff --check
```

Result: diff check passed. The workspace already contained many unrelated dirty files before this task, so the final status is not limited to this plan. No files were staged and no secrets were written.

- [x] **Step 6: Final commit if verification required fixes**

If verification fixes were needed:

```bash
git add <fixed-files>
git commit -m "fix: complete agent ui registry verification"
```

Final commit skipped because the workspace was already dirty with unrelated changes and the user did not request a commit.

### Continuation Verification: 2026-05-17

- [x] Rechecked task list: no unchecked implementation steps remain.
- [x] Re-ran focused Go tests:

```bash
go test ./internal/store ./internal/appui ./internal/server -run 'UICard|AgentUIArtifact|ResourceRoutes' -count=1
```

Result: pass.

- [x] Re-ran focused frontend tests:

```bash
cd web
npm test -- --run src/api/uiCards.test.js src/api/agentUiArtifacts.test.ts src/lib/agentUiCardRegistry.test.tsx src/components/chat/AgentUiArtifactPart.test.tsx src/pages/UICardManagementPage.test.tsx src/pages/AgentUICenterPage.test.tsx src/router.test.tsx
```

Result: pass. jsdom emitted the known non-fatal `HTMLCanvasElement.getContext` warning.

- [x] Re-ran frontend build:

```bash
cd web
npm run build
```

Result: pass. Vite emitted the existing chunk-size warning only.

- [x] Re-ran Playwright UI smoke test:

```bash
cd web
npm run test:ui -- e2e/agent-ui-registry.spec.js --project=chromium
```

Result: pass.

- [x] Re-ran diff hygiene check:

```bash
git diff --check
git status --short
```

Result: diff check passed. The repository remains dirty with many existing unrelated changes; no files were staged.

---

## Fresh Verification Notes - 2026-05-17

- Rechecked this task list while validating the Coroot RCA MCP-first implementation: no unchecked implementation steps remain.
- Agent UI registry Playwright suite passed: `npm run test:ui -- e2e/agent-ui-registry.spec.js --project=chromium`.
- Frontend focused registry/artifact tests passed as part of: `npm test -- --run src/components/chat/rca/rcaReportModel.test.ts src/components/chat/RCAReportArtifact.test.tsx src/components/chat/AgentUiArtifactPart.test.tsx src/lib/agentUiCardRegistry.test.tsx src/api/agentUiArtifacts.test.ts`.
- Browser-in-app verification used a temporary current-code server at `http://127.0.0.1:18280`: `/ui-cards` showed the built-in card registry including `rca_report`; `/?verify=ops-manual-4field-form` showed the lightweight ops manual artifact without the old extra source label, and its `查看工作流` / `查看手册` controls opened read-only details.

---

## Milestone Acceptance Criteria

### Backend Accepted

- `/api/v1/ui-cards` returns `items`, `stats`, and `total`.
- Built-in definitions seed when the store is empty.
- UI cards can be listed, fetched, updated, status-updated, versioned, previewed, and validated.
- Built-in cards cannot be deleted.
- Dangerous artifact keys are rejected or reported by validation.

### Frontend Accepted

- `/ui-cards` is a real registry page, not only a debug table.
- Existing chat artifact types still render.
- Registry lookup controls renderer selection.
- Unsupported and invalid artifacts show registry-owned terminal cards.
- Preview supports normal, redacted, and restricted contexts.

### Agent UI Center Accepted

- `/agent-ui` lists recent artifacts.
- Filters work for source, type, status, case, and prompt trace.
- Detail drawer shows rendered preview, normalized JSON, metadata, actions, and trace links.
- No raw HTML or script payload executes.

### Quality Accepted

- Focused Go tests pass.
- Focused Vitest tests pass.
- Vite build passes.
- Playwright smoke test passes.
- Implementation keeps current Chat, Coroot, Runner, Prompt Trace, and ops manual artifact behavior compatible.

## Recommended Execution Order

1. Task 1: Store model.
2. Task 2: Domain service.
3. Task 3: UI card HTTP API.
4. Task 5: Frontend UI card API client.
5. Task 6: Normalizer hardening.
6. Task 7: Frontend registry runtime.
7. Task 8: Chat rendering through registry.
8. Task 9: UI card management page.
9. Task 4: Agent UI artifact feed API.
10. Task 10: Agent UI Center page.
11. Task 11: Backend/frontend built-in definition parity.
12. Task 12: End-to-end coverage.
13. Task 13: Full verification pass.

This order gets the registry control plane working before changing Chat rendering, then adds the cross-page artifact feed after the core registry path is stable.
