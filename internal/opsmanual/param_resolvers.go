package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ParamResolver interface {
	Name() string
	Resolve(ctx context.Context, req ParamResolverRequest) (ParamResolverResult, error)
}

type ParamResolverRequest struct {
	Requirement       ParamRequirement
	OperationFrame    OperationFrame
	Manual            OpsManual
	Ledger            OperationContextLedger
	AlreadyResolved   map[string]ResolvedParam
	RunRecords        []RunRecord
	ResourceDiscovery ResourceDiscovery
}

type ParamResolverResult struct {
	Candidates []ParamCandidate `json:"candidates,omitempty"`
	Message    string           `json:"message,omitempty"`
}

type ParamResolverRegistry struct {
	resolvers []ParamResolver
	discovery ResourceDiscovery
	timeout   time.Duration
}

func NewDefaultParamResolverRegistry(discovery ResourceDiscovery) ParamResolverRegistry {
	if discovery == nil {
		discovery = noopResourceDiscovery{}
	}
	return ParamResolverRegistry{
		discovery: discovery,
		timeout:   3 * time.Second,
		resolvers: []ParamResolver{
			selectedHostResolver{},
			explicitResourceMatchResolver{},
			conversationResolver{},
			manualDefaultResolver{},
			runRecordResolver{},
			corootServiceResolver{},
			hostReadonlyResolver{},
			dockerResourceResolver{},
			k8sResourceResolver{},
		},
	}
}

func (r ParamResolverRegistry) Names() []string {
	out := make([]string, 0, len(r.resolvers))
	for _, resolver := range r.resolvers {
		out = append(out, resolver.Name())
	}
	return out
}

func (r ParamResolverRegistry) Resolve(ctx context.Context, req ParamResolverRequest) (ParamResolverResult, []ParamResolverLog) {
	if req.ResourceDiscovery == nil {
		req.ResourceDiscovery = r.discovery
	}
	var combined ParamResolverResult
	var logs []ParamResolverLog
	for _, resolver := range r.resolvers {
		started := time.Now().UTC()
		resolveCtx, cancel := context.WithTimeout(ctx, r.timeout)
		result, err := resolver.Resolve(resolveCtx, req)
		cancel()
		log := ParamResolverLog{
			Resolver:  resolver.Name(),
			Status:    "miss",
			StartedAt: started.Format(time.RFC3339),
			EndedAt:   time.Now().UTC().Format(time.RFC3339),
		}
		if err != nil {
			log.Status = "error"
			log.Message = err.Error()
			logs = append(logs, log)
			continue
		}
		if result.Message != "" {
			log.Message = result.Message
			if combined.Message == "" {
				combined.Message = result.Message
			}
		}
		if len(result.Candidates) > 0 {
			log.Status = "hit"
			combined.Candidates = append(combined.Candidates, result.Candidates...)
			logs = append(logs, log)
			if hasHighConfidenceUnique(result.Candidates) {
				return ParamResolverResult{Candidates: result.Candidates, Message: result.Message}, logs
			}
			continue
		}
		logs = append(logs, log)
	}
	combined.Candidates = dedupeParamCandidates(combined.Candidates)
	return combined, logs
}

func hasHighConfidenceUnique(candidates []ParamCandidate) bool {
	return len(candidates) == 1 && candidates[0].Confidence >= 0.85
}

type selectedHostResolver struct{}

func (selectedHostResolver) Name() string { return "selected_host" }

func (selectedHostResolver) Resolve(_ context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if NormalizeParamType(req.Requirement.ID, req.Requirement.Type) != "host_ref" {
		return ParamResolverResult{}, nil
	}
	if fact, ok := req.Ledger.Find("target_host"); ok {
		return ParamResolverResult{Candidates: []ParamCandidate{candidateFromFact(fact)}}, nil
	}
	if host := firstHostFromFrame(req.OperationFrame); host != "" {
		return ParamResolverResult{Candidates: []ParamCandidate{{Value: host, Label: host, Source: "operation_frame", Confidence: 0.86, Evidence: "operation_frame target host"}}}, nil
	}
	return ParamResolverResult{}, nil
}

type conversationResolver struct{}

func (conversationResolver) Name() string { return "conversation" }

func (conversationResolver) Resolve(_ context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if req.Requirement.Sensitive || NormalizeParamType(req.Requirement.ID, req.Requirement.Type) == "secret_ref" {
		if fact, ok := req.Ledger.FindByType(req.Requirement.ID); ok && fact.ConfirmedByUser && fact.Confidence >= 0.95 {
			return ParamResolverResult{Candidates: []ParamCandidate{candidateFromFact(fact)}}, nil
		}
		return ParamResolverResult{}, nil
	}
	if fact, ok := req.Ledger.FindByType(req.Requirement.ID); ok {
		return ParamResolverResult{Candidates: []ParamCandidate{candidateFromFact(fact)}}, nil
	}
	return ParamResolverResult{}, nil
}

type explicitResourceMatchResolver struct{}

func (explicitResourceMatchResolver) Name() string { return "explicit_resource_match" }

func (explicitResourceMatchResolver) Resolve(ctx context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if NormalizeParamType(req.Requirement.ID, req.Requirement.Type) != "resource_ref" {
		return ParamResolverResult{}, nil
	}
	if req.ResourceDiscovery == nil {
		return ParamResolverResult{}, nil
	}
	resources, err := req.ResourceDiscovery.DiscoverHostResources(ctx, resolvedHost(req))
	if err != nil {
		return ParamResolverResult{}, err
	}
	candidates := resourceCandidatesForRequirement(resources, req)
	if len(candidates) == 0 {
		return ParamResolverResult{}, nil
	}
	matched := filterExplicitResourceCandidates(candidates, req)
	if len(matched) == 1 {
		candidate := matched[0]
		if candidate.Confidence < 0.9 {
			candidate.Confidence = 0.9
		}
		candidate.Source = firstNonEmpty(candidate.Source, "resource_discovery")
		candidate.Evidence = firstNonEmpty(candidate.Evidence, "explicit target matched read-only resource discovery")
		if !strings.Contains(candidate.Evidence, "explicit target") {
			candidate.Evidence = "explicit target matched read-only resource discovery; " + candidate.Evidence
		}
		return ParamResolverResult{Candidates: []ParamCandidate{candidate}}, nil
	}
	return ParamResolverResult{}, nil
}

type manualDefaultResolver struct{}

func (manualDefaultResolver) Name() string { return "manual_default" }

func (manualDefaultResolver) Resolve(_ context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if req.Requirement.DefaultValue == nil {
		return ParamResolverResult{}, nil
	}
	label := strings.TrimSpace(fmt.Sprint(req.Requirement.DefaultValue))
	return ParamResolverResult{Candidates: []ParamCandidate{{Value: req.Requirement.DefaultValue, Label: label, Source: "manual_default", Confidence: 0.8, Evidence: "manual default value"}}}, nil
}

type runRecordResolver struct{}

func (runRecordResolver) Name() string { return "run_record" }

func (runRecordResolver) Resolve(_ context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	for _, record := range req.RunRecords {
		if valuePresent(record.RedactedParameters[req.Requirement.ID]) {
			value := record.RedactedParameters[req.Requirement.ID]
			return ParamResolverResult{Candidates: []ParamCandidate{{Value: value, Label: fmt.Sprint(value), Source: "run_record", Confidence: 0.72, Evidence: "previous successful run record"}}}, nil
		}
	}
	return ParamResolverResult{}, nil
}

type corootServiceResolver struct{}

func (corootServiceResolver) Name() string { return "coroot" }

func (corootServiceResolver) Resolve(context.Context, ParamResolverRequest) (ParamResolverResult, error) {
	return ParamResolverResult{}, nil
}

type hostReadonlyResolver struct{}

func (hostReadonlyResolver) Name() string { return "host_readonly" }

func (hostReadonlyResolver) Resolve(ctx context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if NormalizeParamType(req.Requirement.ID, req.Requirement.Type) != "execution_surface" {
		return ParamResolverResult{}, nil
	}
	if surface := surfaceFromResolvedResource(req); surface != "" {
		return ParamResolverResult{Candidates: []ParamCandidate{{
			Value:      surface,
			Label:      surface,
			Source:     "resource_discovery",
			Confidence: 0.9,
			Evidence:   "execution surface from resolved target resource",
		}}}, nil
	}
	host := resolvedHost(req)
	if host == "" {
		return ParamResolverResult{}, nil
	}
	if discovery := req.ResourceDiscovery; discovery != nil {
		surfaces, err := discovery.DiscoverExecutionSurfaces(ctx, host)
		if err != nil {
			return ParamResolverResult{}, err
		}
		if len(surfaces) > 0 {
			return ParamResolverResult{Candidates: dedupeParamCandidates(surfaces)}, nil
		}
	}
	return ParamResolverResult{Candidates: []ParamCandidate{{Value: "ssh", Label: "ssh", Source: "host_readonly", Confidence: 0.78, Evidence: "host selected for read-only access"}}}, ctx.Err()
}

func surfaceFromResolvedResource(req ParamResolverRequest) string {
	if req.AlreadyResolved == nil {
		return ""
	}
	resolved, ok := req.AlreadyResolved["target_instance"]
	if !ok || !valuePresent(resolved.Value) {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(resolved.Value))
	if value == "" || req.ResourceDiscovery == nil {
		return ""
	}
	resources, err := req.ResourceDiscovery.DiscoverHostResources(context.Background(), resolvedHost(req))
	if err != nil {
		return ""
	}
	for _, resource := range resources {
		if strings.EqualFold(strings.TrimSpace(resource.ID), value) ||
			strings.EqualFold(strings.TrimSpace(resource.Name), value) {
			return strings.TrimSpace(resource.Surface)
		}
	}
	return ""
}

type dockerResourceResolver struct{}

func (dockerResourceResolver) Name() string { return "docker" }

func (dockerResourceResolver) Resolve(ctx context.Context, req ParamResolverRequest) (ParamResolverResult, error) {
	if NormalizeParamType(req.Requirement.ID, req.Requirement.Type) != "resource_ref" {
		return ParamResolverResult{}, nil
	}
	discovery := req.ResourceDiscovery
	if discovery == nil {
		return ParamResolverResult{}, nil
	}
	resources, err := discovery.DiscoverHostResources(ctx, resolvedHost(req))
	if err != nil {
		return ParamResolverResult{}, err
	}
	out := resourceCandidatesForRequirement(resources, req)
	if len(out) == 0 {
		objectType := strings.TrimSpace(firstNonEmpty(req.OperationFrame.ObjectType, req.OperationFrame.Target.Type, req.Manual.Operation.TargetType, req.Manual.Applicability.Middleware))
		return ParamResolverResult{Message: noResourceCandidateMessage(req, resources, objectType)}, nil
	}
	return ParamResolverResult{Candidates: out}, nil
}

func resourceCandidatesForRequirement(resources []ResourceCandidate, req ParamResolverRequest) []ParamCandidate {
	objectType := strings.TrimSpace(firstNonEmpty(req.OperationFrame.ObjectType, req.OperationFrame.Target.Type, req.Manual.Operation.TargetType, req.Manual.Applicability.Middleware))
	var out []ParamCandidate
	for _, resource := range resources {
		if !resourceMatchesManualApplicability(resource, req.Manual) {
			continue
		}
		if objectType != "" && !strings.EqualFold(strings.TrimSpace(resource.Type), objectType) {
			continue
		}
		value := firstNonEmpty(resource.ID, resource.Name)
		if value == "" {
			continue
		}
		confidence := resource.Confidence
		if confidence == 0 {
			confidence = 0.86
		}
		source := firstNonEmpty(resource.Source, "docker")
		out = append(out, ParamCandidate{
			Value:      value,
			Label:      firstNonEmpty(resource.Name, value),
			Hint:       strings.TrimSpace(resource.Surface),
			Source:     source,
			Confidence: confidence,
			Evidence:   firstNonEmpty(resource.Evidence, "read-only resource discovery"),
		})
	}
	return out
}

type k8sResourceResolver struct{}

func (k8sResourceResolver) Name() string { return "k8s" }

func (k8sResourceResolver) Resolve(context.Context, ParamResolverRequest) (ParamResolverResult, error) {
	return ParamResolverResult{}, nil
}

func resourceMatchesManualApplicability(resource ResourceCandidate, manual OpsManual) bool {
	if len(manual.Applicability.Platform) > 0 {
		resourcePlatform := resourcePlatform(resource)
		if resourcePlatform != "" && !stringSliceContainsFold(manual.Applicability.Platform, resourcePlatform) {
			return false
		}
	}
	if len(manual.Applicability.ExecutionSurface) > 0 {
		resourceSurface := resourceExecutionSurface(resource)
		if resourceSurface != "" && !stringSliceContainsFold(manual.Applicability.ExecutionSurface, resourceSurface) {
			return false
		}
	}
	return true
}

func resourcePlatform(resource ResourceCandidate) string {
	switch strings.ToLower(strings.TrimSpace(resource.Source)) {
	case "k8s", "kubernetes":
		return "kubernetes"
	case "docker", "host_readonly":
		return "vm"
	default:
		return ""
	}
}

func resourceExecutionSurface(resource ResourceCandidate) string {
	source := strings.ToLower(strings.TrimSpace(resource.Source))
	surface := strings.ToLower(strings.TrimSpace(resource.Surface))
	switch {
	case source == "k8s" || source == "kubernetes" || strings.HasPrefix(surface, "kubectl"):
		return "kubectl"
	case source == "docker" || strings.HasPrefix(surface, "docker exec"):
		return "ssh"
	case source == "host_readonly" || surface == "local shell" || strings.HasPrefix(surface, "ssh"):
		return "ssh"
	default:
		return ""
	}
}

func stringSliceContainsFold(items []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}

func candidateFromFact(fact OperationContextFact) ParamCandidate {
	return ParamCandidate{
		Value:      fact.Value,
		Label:      fmt.Sprint(fact.Value),
		Source:     fact.Source,
		Confidence: fact.Confidence,
		Evidence:   "context fact: " + fact.Key,
	}
}

func filterExplicitResourceCandidates(candidates []ParamCandidate, req ParamResolverRequest) []ParamCandidate {
	needles := explicitResourceNeedles(req, candidates)
	if len(needles) == 0 {
		return nil
	}
	var out []ParamCandidate
	for _, candidate := range candidates {
		if candidateMatchesExplicitNeedle(candidate, needles) {
			out = append(out, candidate)
		}
	}
	return dedupeParamCandidates(out)
}

func explicitResourceNeedles(req ParamResolverRequest, candidates []ParamCandidate) []string {
	var out []string
	add := func(value any) {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" || text == "<nil>" {
			return
		}
		out = appendUnique(out, text)
	}
	if fact, ok := req.Ledger.Find(req.Requirement.ID); ok {
		add(fact.Value)
	}
	if fact, ok := req.Ledger.Find("target_instance"); ok {
		add(fact.Value)
	}
	switch strings.TrimSpace(req.OperationFrame.ObjectType) {
	case "redis":
		if fact, ok := req.Ledger.Find("redis_instance"); ok {
			add(fact.Value)
		}
	case "postgresql":
		if fact, ok := req.Ledger.Find("pg_instance"); ok {
			add(fact.Value)
		}
	case "mysql":
		if fact, ok := req.Ledger.Find("mysql_instance"); ok {
			add(fact.Value)
		}
	}
	add(req.OperationFrame.Target.Name)
	rawText := strings.TrimSpace(req.OperationFrame.RawText)
	if rawText != "" {
		for _, candidate := range candidates {
			if resourceIdentifierMentioned(rawText, fmt.Sprint(candidate.Value)) || resourceIdentifierMentioned(rawText, candidate.Label) {
				add(candidate.Value)
				add(candidate.Label)
			}
		}
	}
	return out
}

func candidateMatchesExplicitNeedle(candidate ParamCandidate, needles []string) bool {
	value := strings.TrimSpace(fmt.Sprint(candidate.Value))
	label := strings.TrimSpace(candidate.Label)
	for _, needle := range needles {
		needle = strings.TrimSpace(needle)
		if needle == "" {
			continue
		}
		for _, candidateValue := range []string{value, label, strings.TrimPrefix(value, "docker:"), strings.TrimPrefix(value, "host:"), strings.TrimPrefix(value, "k8s:pod:")} {
			if candidateValue != "" && strings.EqualFold(candidateValue, needle) {
				return true
			}
		}
	}
	return false
}

func resourceIdentifierMentioned(text, identifier string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if text == "" || identifier == "" || identifier == "<nil>" {
		return false
	}
	if strings.HasPrefix(identifier, "docker:") {
		identifier = strings.TrimPrefix(identifier, "docker:")
	}
	if strings.HasPrefix(identifier, "host:") {
		parts := strings.Split(identifier, ":")
		identifier = parts[len(parts)-1]
	}
	if strings.HasPrefix(identifier, "k8s:pod:") {
		parts := strings.Split(identifier, "/")
		identifier = parts[len(parts)-1]
	}
	if identifier == "" {
		return false
	}
	index := strings.Index(text, identifier)
	if index < 0 {
		return false
	}
	beforeOK := index == 0 || isResourceBoundary(rune(text[index-1]))
	afterIndex := index + len(identifier)
	afterOK := afterIndex >= len(text) || isResourceBoundary(rune(text[afterIndex]))
	return beforeOK && afterOK
}

func isResourceBoundary(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= '0' && r <= '9':
		return false
	case r == '_' || r == '-' || r == '.':
		return false
	default:
		return true
	}
}

func resolvedHost(req ParamResolverRequest) string {
	if req.AlreadyResolved != nil {
		if resolved, ok := req.AlreadyResolved["target_host"]; ok && valuePresent(resolved.Value) {
			return strings.TrimSpace(fmt.Sprint(resolved.Value))
		}
	}
	if fact, ok := req.Ledger.Find("target_host"); ok {
		return strings.TrimSpace(fmt.Sprint(fact.Value))
	}
	return firstHostFromFrame(req.OperationFrame)
}

func noResourceCandidateMessage(req ParamResolverRequest, resources []ResourceCandidate, objectType string) string {
	host := firstNonEmpty(resolvedHost(req), "current host")
	displayType := displayObjectType(firstNonEmpty(objectType, req.Manual.Operation.TargetType, req.Manual.Applicability.Middleware, "target"))
	if len(resources) == 0 {
		return fmt.Sprintf("No %s resource was discovered on %s by read-only resource discovery.", displayType, host)
	}
	return fmt.Sprintf("Read-only resource discovery ran on %s, but found no %s candidate.", host, displayType)
}

func dedupeParamCandidates(candidates []ParamCandidate) []ParamCandidate {
	out := []ParamCandidate{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		key := fmt.Sprint(candidate.Value)
		if strings.TrimSpace(key) == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, candidate)
	}
	return out
}
