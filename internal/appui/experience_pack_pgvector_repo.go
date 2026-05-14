package appui

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const experiencePackEmbeddingDimensions = 16

var (
	_ ExperiencePackRepository        = (*PGVectorExperiencePackRepository)(nil)
	_ ExperiencePackIndexedRepository = (*PGVectorExperiencePackRepository)(nil)
)

type PGVectorExperiencePackRepository struct {
	db *sql.DB
}

func NewPGVectorExperiencePackRepository(dsn string) (*PGVectorExperiencePackRepository, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("pgvector experience pack dsn is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	repo := &PGVectorExperiencePackRepository{db: db}
	if err := repo.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return repo, nil
}

func (r *PGVectorExperiencePackRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *PGVectorExperiencePackRepository) migrate(ctx context.Context) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("pgvector repository is not initialized")
	}
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		`CREATE TABLE IF NOT EXISTS aiops_experience_pack_docs (
			pack_id text PRIMARY KEY,
			candidate_id text NOT NULL DEFAULT '',
			title text NOT NULL DEFAULT '',
			summary text NOT NULL DEFAULT '',
			middleware text NOT NULL DEFAULT '',
			category text NOT NULL DEFAULT '',
			usage_shape text NOT NULL DEFAULT '',
			status text NOT NULL DEFAULT '',
			review_status text NOT NULL DEFAULT '',
			enabled boolean NOT NULL DEFAULT false,
			has_runner_binding boolean NOT NULL DEFAULT false,
			search_doc text NOT NULL DEFAULT '',
			environment_doc text NOT NULL DEFAULT '',
			payload jsonb NOT NULL,
			embedding vector(16) NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS aiops_experience_pack_candidates (
			candidate_id text PRIMARY KEY,
			pack_id text NOT NULL DEFAULT '',
			status text NOT NULL DEFAULT '',
			payload jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS aiops_experience_pack_reuse_records (
			id text PRIMARY KEY,
			pack_id text NOT NULL DEFAULT '',
			payload jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS aiops_experience_pack_docs_lookup_idx ON aiops_experience_pack_docs (review_status, enabled, middleware, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS aiops_experience_pack_docs_search_idx ON aiops_experience_pack_docs USING gin (to_tsvector('simple', search_doc))`,
		`CREATE INDEX IF NOT EXISTS aiops_experience_pack_candidates_pack_idx ON aiops_experience_pack_candidates (pack_id, updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	_, _ = r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS aiops_experience_pack_docs_embedding_hnsw ON aiops_experience_pack_docs USING hnsw (embedding vector_cosine_ops)`)
	return nil
}

func (r *PGVectorExperiencePackRepository) ListExperiencePacks(req ListExperiencePacksRequest) (ExperiencePackLibraryList, error) {
	if r == nil || r.db == nil {
		return ExperiencePackLibraryList{}, fmt.Errorf("pgvector repository is not initialized")
	}
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	args := []any{}
	clauses := []string{"1=1"}
	add := func(condition string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(condition, len(args)))
	}
	if strings.TrimSpace(req.Status) != "" {
		add("status = $%d", strings.TrimSpace(req.Status))
	}
	if strings.TrimSpace(req.Category) != "" {
		add("category = $%d", strings.TrimSpace(req.Category))
	}
	if strings.TrimSpace(req.UsageShape) != "" {
		add("usage_shape = $%d", strings.TrimSpace(req.UsageShape))
	}
	if strings.TrimSpace(req.Middleware) != "" {
		args = append(args, strings.ToLower(strings.TrimSpace(req.Middleware)))
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf("(middleware = $%d OR search_doc ILIKE '%%' || $%d || '%%')", idx, idx))
	}
	if strings.TrimSpace(req.HasRunnerBinding) == "true" {
		clauses = append(clauses, "has_runner_binding = true")
	}
	args = append(args, limit)
	query := fmt.Sprintf(`SELECT payload FROM aiops_experience_pack_docs WHERE %s ORDER BY updated_at DESC LIMIT $%d`, strings.Join(clauses, " AND "), len(args))
	rows, err := r.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return ExperiencePackLibraryList{}, err
	}
	defer rows.Close()
	items, err := scanExperiencePackRows(rows)
	if err != nil {
		return ExperiencePackLibraryList{}, err
	}
	return ExperiencePackLibraryList{Items: items, Total: len(items)}, nil
}

func (r *PGVectorExperiencePackRepository) ListExperiencePackCandidates(req ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error) {
	if r == nil || r.db == nil {
		return ExperiencePackCandidateList{}, fmt.Errorf("pgvector repository is not initialized")
	}
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := r.db.QueryContext(context.Background(), `SELECT payload FROM aiops_experience_pack_candidates ORDER BY updated_at DESC LIMIT $1`, limit)
	if err != nil {
		return ExperiencePackCandidateList{}, err
	}
	defer rows.Close()
	items := []ExperiencePackCandidate{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return ExperiencePackCandidateList{}, err
		}
		var candidate ExperiencePackCandidate
		if err := json.Unmarshal(raw, &candidate); err != nil {
			return ExperiencePackCandidateList{}, err
		}
		items = append(items, cloneExperiencePackCandidate(candidate))
	}
	if err := rows.Err(); err != nil {
		return ExperiencePackCandidateList{}, err
	}
	return ExperiencePackCandidateList{Items: items, Total: len(items)}, nil
}

func (r *PGVectorExperiencePackRepository) SaveExperiencePackCandidate(candidate ExperiencePackCandidate) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("pgvector repository is not initialized")
	}
	candidate = cloneExperiencePackCandidate(candidate)
	candidate.ExperiencePack = nil
	now := time.Now().UTC().Format(time.RFC3339)
	if candidate.CreatedAt == "" {
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now
	raw, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(context.Background(), `
		INSERT INTO aiops_experience_pack_candidates (candidate_id, pack_id, status, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, now(), now())
		ON CONFLICT (candidate_id) DO UPDATE SET
			pack_id = EXCLUDED.pack_id,
			status = EXCLUDED.status,
			payload = EXCLUDED.payload,
			updated_at = now()
	`, firstNonEmptyExperiencePackString(candidate.ID, candidate.CandidateID), candidate.PackID, candidate.Status, string(raw))
	return err
}

func (r *PGVectorExperiencePackRepository) GetExperiencePackCandidate(candidateID string) (ExperiencePackCandidate, error) {
	if r == nil || r.db == nil {
		return ExperiencePackCandidate{}, fmt.Errorf("pgvector repository is not initialized")
	}
	target := strings.TrimSpace(candidateID)
	var raw []byte
	err := r.db.QueryRowContext(context.Background(), `
		SELECT payload FROM aiops_experience_pack_candidates
		WHERE candidate_id = $1 OR pack_id = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`, target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ExperiencePackCandidate{}, ErrExperiencePackCandidateNotFound
	}
	if err != nil {
		return ExperiencePackCandidate{}, err
	}
	var candidate ExperiencePackCandidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return ExperiencePackCandidate{}, err
	}
	return cloneExperiencePackCandidate(candidate), nil
}

func (r *PGVectorExperiencePackRepository) GetExperiencePack(packID string) (ExperiencePack, error) {
	if r == nil || r.db == nil {
		return ExperiencePack{}, fmt.Errorf("pgvector repository is not initialized")
	}
	target := strings.TrimSpace(packID)
	var raw []byte
	err := r.db.QueryRowContext(context.Background(), `
		SELECT payload FROM aiops_experience_pack_docs
		WHERE pack_id = $1 OR (payload->>'pack_id') = $1
		LIMIT 1
	`, target).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ExperiencePack{}, ErrExperiencePackNotFound
	}
	if err != nil {
		return ExperiencePack{}, err
	}
	var pack ExperiencePack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return ExperiencePack{}, err
	}
	return cloneExperiencePack(normalizeExperiencePack(pack)), nil
}

func (r *PGVectorExperiencePackRepository) SaveExperiencePack(pack ExperiencePack) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("pgvector repository is not initialized")
	}
	pack = normalizeExperiencePack(pack)
	now := time.Now().UTC().Format(time.RFC3339)
	if pack.CreatedAt == "" {
		pack.CreatedAt = now
	}
	pack.UpdatedAt = now
	raw, err := json.Marshal(pack)
	if err != nil {
		return err
	}
	searchDoc := experiencePackSearchDocument(pack)
	environmentDoc := experiencePackEnvironmentDocument(pack)
	embedding := experiencePackVectorLiteral(experiencePackEmbedding(searchDoc, experiencePackSignalsFromText(searchDoc)))
	_, err = r.db.ExecContext(context.Background(), `
		INSERT INTO aiops_experience_pack_docs (
			pack_id, title, summary, middleware, category, usage_shape, status, review_status,
			enabled, has_runner_binding, search_doc, environment_doc, payload, embedding, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14::vector, now(), now())
		ON CONFLICT (pack_id) DO UPDATE SET
			title = EXCLUDED.title,
			summary = EXCLUDED.summary,
			middleware = EXCLUDED.middleware,
			category = EXCLUDED.category,
			usage_shape = EXCLUDED.usage_shape,
			status = EXCLUDED.status,
			review_status = EXCLUDED.review_status,
			enabled = EXCLUDED.enabled,
			has_runner_binding = EXCLUDED.has_runner_binding,
			search_doc = EXCLUDED.search_doc,
			environment_doc = EXCLUDED.environment_doc,
			payload = EXCLUDED.payload,
			embedding = EXCLUDED.embedding,
			updated_at = now()
	`, pack.ID, pack.Title, pack.Summary, strings.ToLower(pack.Middleware), pack.Category, pack.UsageShape, pack.Status, pack.ReviewStatus, pack.Enabled, packHasExecutableRunnerBinding(pack), searchDoc, environmentDoc, string(raw), embedding)
	return err
}

func (r *PGVectorExperiencePackRepository) ListExperiencePackReuseRecords(packID string, req ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error) {
	if r == nil || r.db == nil {
		return ExperiencePackReuseRecordList{}, fmt.Errorf("pgvector repository is not initialized")
	}
	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := r.db.QueryContext(context.Background(), `SELECT payload FROM aiops_experience_pack_reuse_records WHERE pack_id = $1 ORDER BY created_at DESC LIMIT $2`, strings.TrimSpace(packID), limit)
	if err != nil {
		return ExperiencePackReuseRecordList{}, err
	}
	defer rows.Close()
	items := []ExperiencePackReuseRecord{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return ExperiencePackReuseRecordList{}, err
		}
		var item ExperiencePackReuseRecord
		if err := json.Unmarshal(raw, &item); err != nil {
			return ExperiencePackReuseRecordList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ExperiencePackReuseRecordList{}, err
	}
	return ExperiencePackReuseRecordList{Items: items, Total: len(items)}, nil
}

func (r *PGVectorExperiencePackRepository) RetrieveExperiencePacks(req ExperiencePackRetrieveRequest) (ExperiencePackMatchList, error) {
	if r == nil || r.db == nil {
		return ExperiencePackMatchList{}, fmt.Errorf("pgvector repository is not initialized")
	}
	text := strings.TrimSpace(firstNonEmptyExperiencePackString(req.UserText, req.Query))
	signals := signalsFromAny(req.Signals)
	if text != "" {
		signals = appendExperiencePackSignals(signals, experiencePackSignalsFromText(text)...)
	}
	queryText := strings.TrimSpace(strings.Join(append([]string{text}, signals...), " "))
	if queryText == "" {
		queryText = "experience pack"
	}
	reqProfile := experiencePackProfileFromRequest(req, text, signals)
	vector := experiencePackVectorLiteral(experiencePackEmbedding(queryText, signals))
	limit := 50
	rows, err := r.db.QueryContext(context.Background(), `
		SELECT payload,
			(1 - (embedding <=> $1::vector)) AS vector_score,
			ts_rank_cd(to_tsvector('simple', search_doc), plainto_tsquery('simple', $2)) AS keyword_score,
			search_doc,
			environment_doc
		FROM aiops_experience_pack_docs
		WHERE review_status = 'approved'
			AND enabled = true
			AND ($4 = '' OR middleware = $4 OR search_doc ILIKE '%' || $4 || '%')
		ORDER BY ((1 - (embedding <=> $1::vector)) * 0.65
			+ LEAST(ts_rank_cd(to_tsvector('simple', search_doc), plainto_tsquery('simple', $2)), 1.0) * 0.25) DESC,
			updated_at DESC
		LIMIT $3
	`, vector, queryText, limit, reqProfile.Middleware)
	if err != nil {
		return ExperiencePackMatchList{}, err
	}
	defer rows.Close()
	matches := []ExperiencePackMatch{}
	for rows.Next() {
		var raw []byte
		var vectorScore, keywordScore float64
		var searchDoc, environmentDoc string
		if err := rows.Scan(&raw, &vectorScore, &keywordScore, &searchDoc, &environmentDoc); err != nil {
			return ExperiencePackMatchList{}, err
		}
		var pack ExperiencePack
		if err := json.Unmarshal(raw, &pack); err != nil {
			return ExperiencePackMatchList{}, err
		}
		match, ok := experiencePackMatchFromIndexedPack(normalizeExperiencePack(pack), req, text, signals, searchDoc, environmentDoc, vectorScore, keywordScore)
		if ok {
			matches = append(matches, match)
		}
	}
	if err := rows.Err(); err != nil {
		return ExperiencePackMatchList{}, err
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Confidence > matches[j].Confidence })
	matches = pruneReferenceOnlyExperiencePackMatches(matches)
	if len(matches) > 10 {
		matches = matches[:10]
	}
	return ExperiencePackMatchList{Items: matches, Total: len(matches)}, nil
}

func pruneReferenceOnlyExperiencePackMatches(matches []ExperiencePackMatch) []ExperiencePackMatch {
	hasActionable := false
	for _, match := range matches {
		if match.CompatibilityStatus == "direct" || match.CompatibilityStatus == "adapt_required" || match.CompatibilityStatus == "" {
			hasActionable = true
			break
		}
	}
	if !hasActionable {
		return matches
	}
	pruned := make([]ExperiencePackMatch, 0, len(matches))
	for _, match := range matches {
		if match.CompatibilityStatus == "reference_only" {
			continue
		}
		pruned = append(pruned, match)
	}
	return pruned
}

func scanExperiencePackRows(rows *sql.Rows) ([]ExperiencePack, error) {
	items := []ExperiencePack{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var pack ExperiencePack
		if err := json.Unmarshal(raw, &pack); err != nil {
			return nil, err
		}
		items = append(items, cloneExperiencePack(normalizeExperiencePack(pack)))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func experiencePackMatchFromIndexedPack(pack ExperiencePack, req ExperiencePackRetrieveRequest, text string, signals []string, searchDoc string, environmentDoc string, vectorScore float64, keywordScore float64) (ExperiencePackMatch, bool) {
	searchDocLower := strings.ToLower(searchDoc)
	reqProfile := experiencePackProfileFromRequest(req, text, signals)
	packProfile := experiencePackProfileFromPack(pack, searchDoc, environmentDoc)
	compatibility := assessExperiencePackCompatibility(reqProfile, packProfile)
	if compatibility.Status == "incompatible" {
		return ExperiencePackMatch{}, false
	}
	matched := []string{}
	signalScore := 0.0
	for _, signal := range signals {
		signal = strings.TrimSpace(strings.ToLower(signal))
		if signal == "" {
			continue
		}
		if strings.Contains(searchDocLower, signal) {
			matched = appendExperiencePackSignals(matched, signal)
			signalScore += experiencePackSignalWeight(signal)
		}
	}
	envScore := 0.0
	env := strings.ToLower(strings.TrimSpace(firstNonEmptyExperiencePackString(req.Environment, req.OS)))
	if env != "" && strings.Contains(strings.ToLower(environmentDoc+" "+searchDoc), env) {
		envScore = 0.08
	}
	confidence := vectorScore*0.55 + math.Min(keywordScore, 1)*0.25 + math.Min(signalScore, 0.25) + envScore
	confidence *= compatibility.ConfidenceMultiplier
	if confidence > 0 && compatibility.ConfidencePenalty > 0 {
		confidence -= compatibility.ConfidencePenalty
	}
	if confidence <= 0 {
		return ExperiencePackMatch{}, false
	}
	if compatibility.Status == "reference_only" && confidence < 0.12 {
		return ExperiencePackMatch{}, false
	}
	if confidence < 0.18 && text != "" && len(matched) == 0 {
		return ExperiencePackMatch{}, false
	}
	if confidence > 1 {
		confidence = 1
	}
	reasons := []string{"PostgreSQL pgvector 语义索引命中"}
	switch compatibility.Status {
	case "direct":
		reasons = append(reasons, "硬兼容条件通过")
	case "adapt_required":
		reasons = append(reasons, "硬兼容条件部分匹配，需生成适配计划")
	case "reference_only":
		reasons = append(reasons, "同域参考经验，禁止直接使用 Runner")
	}
	if keywordScore > 0 {
		reasons = append(reasons, "关键词/BM25 全文检索命中")
	}
	if len(matched) > 0 {
		reasons = append(reasons, "GEP Gene signals_match 命中")
	}
	if envScore > 0 {
		reasons = append(reasons, "环境指纹匹配")
	}
	nextActions := []string{"view_skill", "check_preconditions", "view_history", "mark_not_applicable"}
	runner, hasRunner := executableRunnerBindingFromPack(pack)
	switch compatibility.Status {
	case "direct":
		if hasRunner {
			nextActions = append(nextActions, "create_dry_run")
		}
	case "adapt_required":
		nextActions = append(nextActions, "create_adaptation_plan", "generate_runner_variant", "manual_step_approval")
	case "reference_only":
		nextActions = append(nextActions, "use_as_reference", "manual_step_approval")
	}
	if compatibility.Status != "direct" {
		runner = ExperiencePackRunnerBinding{}
	}
	return ExperiencePackMatch{
		PackID: pack.ID, Skill: pack.Skill, Confidence: confidence, CompatibilityStatus: compatibility.Status, CompatibilityGaps: compatibility.Gaps, MatchedSignals: matched,
		MatchReasons: reasons, PreconditionGaps: append([]string(nil), compatibility.Gaps...), RiskWarnings: append([]string(nil), compatibility.Risks...), NextActions: nextActions,
		OSVariant: firstNonEmptyExperiencePackString(req.OS, req.Environment), RunnerBinding: runner, History: pack.History, AdvancedRefs: pack.AdvancedRefs,
	}, true
}

type experiencePackOpsProfile struct {
	Middleware       string
	Operation        string
	OS               string
	ExecutionSurface string
	Tools            []string
	InternetRequired string
}

type experiencePackCompatibilityAssessment struct {
	Status               string
	Gaps                 []string
	Risks                []string
	ConfidenceMultiplier float64
	ConfidencePenalty    float64
}

func experiencePackProfileFromRequest(req ExperiencePackRetrieveRequest, text string, signals []string) experiencePackOpsProfile {
	parts := nonEmptyExperiencePackStrings(text, req.Query, req.Environment, req.OS, strings.Join(signals, " "))
	parts = append(parts, flattenExperiencePackMetadata(req.Metadata)...)
	haystack := strings.ToLower(strings.Join(parts, " "))
	return experiencePackProfileFromText(haystack, req.OS)
}

func experiencePackProfileFromPack(pack ExperiencePack, searchDoc string, environmentDoc string) experiencePackOpsProfile {
	haystack := strings.ToLower(strings.Join(nonEmptyExperiencePackStrings(pack.Middleware, searchDoc, environmentDoc), " "))
	return experiencePackProfileFromText(haystack, "")
}

func experiencePackProfileFromText(haystack string, explicitOS string) experiencePackOpsProfile {
	return experiencePackOpsProfile{
		Middleware:       detectExperiencePackMiddleware(haystack),
		Operation:        detectExperiencePackOperation(haystack),
		OS:               firstNonEmptyExperiencePackString(detectExperiencePackOS(explicitOS), detectExperiencePackOS(haystack)),
		ExecutionSurface: detectExperiencePackExecutionSurface(haystack),
		Tools:            detectExperiencePackTools(haystack),
		InternetRequired: detectExperiencePackInternetRequirement(haystack),
	}
}

func assessExperiencePackCompatibility(req experiencePackOpsProfile, pack experiencePackOpsProfile) experiencePackCompatibilityAssessment {
	next := experiencePackCompatibilityAssessment{Status: "direct", ConfidenceMultiplier: 1}
	if req.Middleware != "" && pack.Middleware != "" && req.Middleware != pack.Middleware {
		next.Status = "incompatible"
		next.Gaps = append(next.Gaps, fmt.Sprintf("中间件不匹配：请求 %s，经验包 %s", req.Middleware, pack.Middleware))
		return next
	}
	if req.Operation != "" && pack.Operation != "" && !experiencePackOperationsCompatible(req.Operation, pack.Operation) {
		next.Status = "reference_only"
		next.ConfidenceMultiplier = 0.35
		next.Gaps = append(next.Gaps, fmt.Sprintf("操作类型不同：请求 %s，经验包 %s", req.Operation, pack.Operation))
		next.Risks = append(next.Risks, "只能参考 Skill 和验证思路，不能使用原 Runner")
		return next
	}
	if req.OS != "" && pack.OS != "" && req.OS != pack.OS {
		next.Status = "adapt_required"
		next.ConfidenceMultiplier = 0.82
		next.Gaps = append(next.Gaps, fmt.Sprintf("操作系统不同：请求 %s，经验包 %s", req.OS, pack.OS))
		next.Risks = append(next.Risks, "需要重写系统包管理、路径和服务管理相关 Runner 节点")
	}
	if req.ExecutionSurface != "" && pack.ExecutionSurface != "" && req.ExecutionSurface != pack.ExecutionSurface {
		next.Status = moreRestrictiveExperiencePackCompatibility(next.Status, "adapt_required")
		next.ConfidenceMultiplier = math.Min(nonZeroExperiencePackFloat(next.ConfidenceMultiplier, 1), 0.78)
		next.Gaps = append(next.Gaps, fmt.Sprintf("执行面不同：请求 %s，经验包 %s", req.ExecutionSurface, pack.ExecutionSurface))
		next.Risks = append(next.Risks, "需要按当前执行面重新生成 Runner 变体")
	}
	if req.InternetRequired == "false" && pack.InternetRequired == "true" {
		next.Status = moreRestrictiveExperiencePackCompatibility(next.Status, "adapt_required")
		next.ConfidenceMultiplier = math.Min(nonZeroExperiencePackFloat(next.ConfidenceMultiplier, 1), 0.72)
		next.Gaps = append(next.Gaps, "网络条件不同：当前请求不允许外网，经验包可能依赖外网下载")
		next.Risks = append(next.Risks, "需要替换为内网源、离线包或预装工具")
	}
	if len(req.Tools) > 0 && len(pack.Tools) > 0 {
		missing := missingExperiencePackTools(req.Tools, pack.Tools)
		if len(missing) > 0 && req.Operation == pack.Operation {
			next.Status = moreRestrictiveExperiencePackCompatibility(next.Status, "adapt_required")
			next.ConfidenceMultiplier = math.Min(nonZeroExperiencePackFloat(next.ConfidenceMultiplier, 1), 0.85)
			next.Gaps = append(next.Gaps, "工具链不同："+strings.Join(missing, "、"))
		}
	}
	if next.Status == "direct" {
		next.Gaps = nil
	}
	if next.ConfidenceMultiplier == 0 {
		next.ConfidenceMultiplier = 1
	}
	return next
}

func moreRestrictiveExperiencePackCompatibility(current, candidate string) string {
	rank := map[string]int{"direct": 0, "adapt_required": 1, "reference_only": 2, "incompatible": 3}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func nonZeroExperiencePackFloat(value float64, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func detectExperiencePackMiddleware(haystack string) string {
	switch {
	case strings.Contains(haystack, "redis") || strings.Contains(haystack, "redis-cli"):
		return "redis"
	case strings.Contains(haystack, "mysql") || strings.Contains(haystack, "mysqldump") || strings.Contains(haystack, "mariadb"):
		return "mysql"
	case strings.Contains(haystack, "postgresql") || strings.Contains(haystack, "postgres") || strings.Contains(haystack, "pg_dump") || strings.Contains(haystack, "pg_basebackup") || strings.Contains(haystack, "pg_mon"):
		return "postgresql"
	case strings.Contains(haystack, "kubernetes") || strings.Contains(haystack, "kubectl") || strings.Contains(haystack, "k8s"):
		return "kubernetes"
	default:
		return ""
	}
}

func detectExperiencePackOperation(haystack string) string {
	switch {
	case strings.Contains(haystack, "pg_basebackup") || strings.Contains(haystack, "replication") || strings.Contains(haystack, "primary standby") || strings.Contains(haystack, "主从") || strings.Contains(haystack, "部署"):
		return "deploy"
	case strings.Contains(haystack, "mysqldump") || strings.Contains(haystack, "pg_dump") || strings.Contains(haystack, "备份") || strings.Contains(haystack, "backup"):
		return "backup"
	case strings.Contains(haystack, "install "):
		return "deploy"
	case strings.Contains(haystack, "p95") || strings.Contains(haystack, "latency") || strings.Contains(haystack, "排查") || strings.Contains(haystack, "故障") || strings.Contains(haystack, "rca"):
		return "rca"
	case strings.Contains(haystack, "maxmemory") || strings.Contains(haystack, "优化") || strings.Contains(haystack, "tune"):
		return "optimize"
	default:
		return ""
	}
}

func detectExperiencePackOS(haystack string) string {
	normalized := strings.ToLower(strings.TrimSpace(haystack))
	switch {
	case strings.Contains(normalized, "ubuntu"):
		return "ubuntu"
	case strings.Contains(normalized, "centos"):
		return "centos"
	case strings.Contains(normalized, "rocky"):
		return "rocky"
	case strings.Contains(normalized, "rhel") || strings.Contains(normalized, "red hat"):
		return "rhel"
	case strings.Contains(normalized, "debian"):
		return "debian"
	case strings.Contains(normalized, "linux"):
		return "linux"
	default:
		return ""
	}
}

func detectExperiencePackExecutionSurface(haystack string) string {
	switch {
	case strings.Contains(haystack, "kubectl") || strings.Contains(haystack, "kubernetes") || strings.Contains(haystack, "k8s"):
		return "kubernetes"
	case strings.Contains(haystack, "docker exec") || strings.Contains(haystack, "docker container") || strings.Contains(haystack, "容器"):
		return "docker"
	case strings.Contains(haystack, "ssh ") || strings.Contains(haystack, "systemctl"):
		return "ssh"
	default:
		return ""
	}
}

func detectExperiencePackTools(haystack string) []string {
	tools := []string{}
	for _, tool := range []string{"mysqldump", "mysql", "pg_dump", "pg_basebackup", "psql", "redis-cli", "kubectl", "helm", "docker", "systemctl", "apt-get", "apt ", "yum", "dnf"} {
		if strings.Contains(haystack, tool) {
			tools = appendExperiencePackSignals(tools, strings.TrimSpace(tool))
		}
	}
	return tools
}

func detectExperiencePackInternetRequirement(haystack string) string {
	switch {
	case strings.Contains(haystack, "无外网") || strings.Contains(haystack, "离线") || strings.Contains(haystack, "no internet") || strings.Contains(haystack, "offline"):
		return "false"
	case strings.Contains(haystack, "apt-get install") || strings.Contains(haystack, "yum install") || strings.Contains(haystack, "dnf install") || strings.Contains(haystack, "curl ") || strings.Contains(haystack, "wget "):
		return "true"
	default:
		return ""
	}
}

func experiencePackOperationsCompatible(left string, right string) bool {
	if left == right {
		return true
	}
	if (left == "backup" && right == "restore") || (left == "restore" && right == "backup") {
		return true
	}
	return false
}

func missingExperiencePackTools(required []string, available []string) []string {
	missing := []string{}
	for _, tool := range required {
		if tool == "" || containsExperiencePackSignal(available, tool) {
			continue
		}
		missing = append(missing, tool)
	}
	return missing
}

func experiencePackSignalWeight(signal string) float64 {
	switch strings.ToLower(strings.TrimSpace(signal)) {
	case "验证", "恢复验证", "恢复", "回滚", "dry run", "approval", "审批":
		return 0.02
	case "mysql", "mysqldump", "postgresql", "postgres", "pg_dump", "pg_basebackup", "redis", "redis-cli", "kubectl", "k8s", "kubernetes":
		return 0.1
	default:
		return 0.06
	}
}

func experiencePackExplicitDomainCompatible(signals []string, haystack string) bool {
	domainSignals := []string{}
	for _, signal := range signals {
		normalized := strings.TrimSpace(strings.ToLower(signal))
		if normalized == "" {
			continue
		}
		switch normalized {
		case "mysql", "mysqldump", "redis", "redis-cli", "postgresql", "postgres", "pg_mon", "pg-primary", "pg-standby", "kubernetes", "kubectl", "k8s":
			domainSignals = append(domainSignals, normalized)
		case "pg":
			if !containsExperiencePackSignal(domainSignals, "postgresql") {
				domainSignals = append(domainSignals, normalized)
			}
		}
	}
	if len(domainSignals) == 0 {
		return true
	}
	for _, signal := range domainSignals {
		switch signal {
		case "postgres", "postgresql", "pg", "pg-primary", "pg-standby", "pg_mon":
			if strings.Contains(haystack, "postgres") || strings.Contains(haystack, "pg_mon") || strings.Contains(haystack, "pg-primary") || strings.Contains(haystack, "pg-standby") {
				return true
			}
		case "mysql", "mysqldump":
			if strings.Contains(haystack, "mysql") || strings.Contains(haystack, "mysqldump") {
				return true
			}
		case "redis", "redis-cli":
			if strings.Contains(haystack, "redis") {
				return true
			}
		case "kubernetes", "kubectl", "k8s":
			if strings.Contains(haystack, "kubernetes") || strings.Contains(haystack, "kubectl") || strings.Contains(haystack, "k8s") {
				return true
			}
		}
	}
	return false
}

func experiencePackSearchDocument(pack ExperiencePack) string {
	parts := []string{
		pack.ID, pack.PackID, pack.Title, pack.Summary, pack.Category, pack.UsageShape, pack.Middleware,
		pack.Skill.ID, pack.Skill.Name, pack.Skill.Summary, pack.Skill.Path,
		strings.Join(pack.Tags, " "),
		pack.AdvancedRefs.GeneAssetID,
		strings.Join(pack.AdvancedRefs.CapsuleAssetIDs, " "),
	}
	for _, binding := range pack.RunnerBindings {
		parts = append(parts, binding.WorkflowID, binding.WorkflowName, binding.Status, binding.ReviewStatus)
	}
	for _, scope := range pack.AuthorizationScopes {
		parts = append(parts, scope.Type, scope.Value, scope.Reason)
	}
	parts = append(parts, flattenExperiencePackMetadata(pack.Metadata)...)
	return strings.Join(nonEmptyExperiencePackStrings(parts...), " ")
}

func experiencePackEnvironmentDocument(pack ExperiencePack) string {
	parts := []string{pack.Middleware}
	for _, scope := range pack.AuthorizationScopes {
		if strings.EqualFold(scope.Type, "environment") || strings.EqualFold(scope.Type, "service") {
			parts = append(parts, scope.Value)
		}
	}
	if pack.Metadata != nil {
		parts = append(parts, flattenExperiencePackMetadata(pack.Metadata["env_fingerprint"])...)
		parts = append(parts, flattenExperiencePackMetadata(pack.Metadata["environment"])...)
	}
	return strings.Join(nonEmptyExperiencePackStrings(parts...), " ")
}

func flattenExperiencePackMetadata(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return nonEmptyExperiencePackStrings(typed)
	case []string:
		return nonEmptyExperiencePackStrings(typed...)
	case []any:
		out := []string{}
		for _, item := range typed {
			out = append(out, flattenExperiencePackMetadata(item)...)
		}
		return out
	case map[string]any:
		out := []string{}
		for key, item := range typed {
			out = append(out, key)
			out = append(out, flattenExperiencePackMetadata(item)...)
		}
		return out
	default:
		return nonEmptyExperiencePackStrings(fmt.Sprint(typed))
	}
}

func nonEmptyExperiencePackStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func experiencePackEmbedding(text string, signals []string) []float64 {
	tokens := experiencePackEmbeddingTokens(text)
	tokens = append(tokens, experiencePackEmbeddingTokens(strings.Join(signals, " "))...)
	if len(tokens) == 0 {
		tokens = []string{"empty-experience-pack-query"}
	}
	vector := make([]float64, experiencePackEmbeddingDimensions)
	for _, token := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(strings.ToLower(token)))
		sum := h.Sum64()
		dim := int(sum % uint64(experiencePackEmbeddingDimensions))
		weight := 1.0
		if (sum>>8)&1 == 0 {
			weight = -1.0
		}
		vector[dim] += weight
	}
	norm := 0.0
	for _, value := range vector {
		norm += value * value
	}
	if norm == 0 {
		return vector
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] = vector[i] / norm
	}
	return vector
}

func experiencePackEmbeddingTokens(text string) []string {
	normalized := strings.ToLower(text)
	tokens := []string{}
	for _, signal := range experiencePackSignalsFromText(normalized) {
		tokens = append(tokens, signal)
	}
	for _, token := range experiencePackSignalTokenPattern.FindAllString(normalized, -1) {
		if len(token) >= 3 {
			tokens = append(tokens, token)
		}
	}
	for _, marker := range []string{"主从", "部署", "备份", "恢复", "排障", "异常", "验证", "回滚", "监控"} {
		if strings.Contains(normalized, marker) {
			tokens = append(tokens, marker)
		}
	}
	return tokens
}

func experiencePackVectorLiteral(vector []float64) string {
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = fmt.Sprintf("%.8f", value)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
