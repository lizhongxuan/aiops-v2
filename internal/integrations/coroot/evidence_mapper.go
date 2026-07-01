package coroot

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/observability"
)

func MapCorootEvidencePack(project string, payload any) observability.EvidencePack {
	pack := observability.EvidencePack{
		Provider: "coroot",
		Project:  strings.TrimSpace(project),
	}
	if payload == nil {
		pack.MissingEvidence = []string{"Coroot evidence payload is empty"}
		return pack
	}

	root := corootEvidenceMap(payload)
	if len(root) == 0 {
		pack.MissingEvidence = []string{"Coroot evidence payload could not be decoded"}
		return pack
	}
	if pack.Project == "" {
		pack.Project = stringFromAny(root["project"])
	}

	pack.Target = corootEvidenceTarget(root)
	pack.TargetStatus = corootEvidenceStatus(root, pack.Target.Name)
	pack.DependencyEdges = corootEvidenceDependencyEdges(root, pack.Target.Name)
	pack.Incidents = corootEvidenceIncidents(root)
	pack.Metrics = corootEvidenceMetrics(root, pack.Target.Name)
	pack.Logs = corootEvidenceLogs(root)
	pack.Traces = corootEvidenceTraces(root)
	pack.Deployments = corootEvidenceDeployments(root)
	pack.Hypotheses = corootEvidenceHypotheses(root)
	pack.MissingEvidence = corootEvidenceMissing(root)
	return pack
}

func corootEvidenceMap(payload any) map[string]any {
	if typed, ok := payload.(map[string]any); ok {
		return typed
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func objectFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func corootEvidenceTarget(root map[string]any) observability.EntityRef {
	if target := objectFromAny(root["target"]); len(target) > 0 {
		id := firstNonBlank(
			stringFromAny(target["service"]),
			stringFromAny(target["id"]),
			stringFromAny(target["applicationId"]),
			stringFromAny(target["appId"]),
			stringFromAny(target["name"]),
		)
		name := firstNonBlank(
			stringFromAny(target["serviceName"]),
			stringFromAny(target["name"]),
			serviceName(id),
			id,
		)
		return observability.EntityRef{
			Kind:    firstNonBlank(stringFromAny(target["kind"]), "service"),
			ID:      id,
			Name:    name,
			Cluster: stringFromAny(target["cluster"]),
			RawRef:  corootEvidenceRawRef(target),
		}
	}
	id := firstNonBlank(
		stringFromAny(root["service"]),
		stringFromAny(root["applicationId"]),
		stringFromAny(root["appId"]),
		stringFromAny(root["target"]),
	)
	name := firstNonBlank(stringFromAny(root["serviceName"]), stringFromAny(root["name"]), serviceName(id), id)
	if id == "" && name == "" {
		return observability.EntityRef{}
	}
	return observability.EntityRef{
		Kind:    "service",
		ID:      id,
		Name:    name,
		Cluster: stringFromAny(root["cluster"]),
		RawRef:  corootEvidenceRawRef(root),
	}
}

func corootEvidenceStatus(root map[string]any, target string) []observability.StatusEvidence {
	var out []observability.StatusEvidence
	appendStatus := func(entity, name, status, summary, severity, confidence, rawRef string) {
		if entity == "" && name == "" && status == "" && summary == "" {
			return
		}
		out = append(out, observability.StatusEvidence{
			Entity:     firstNonBlank(entity, target),
			Name:       name,
			Status:     status,
			Summary:    summary,
			Severity:   severity,
			Confidence: confidence,
			RawRef:     rawRef,
		})
	}
	if targetObj := objectFromAny(root["target"]); len(targetObj) > 0 {
		appendStatus(
			firstNonBlank(stringFromAny(targetObj["service"]), stringFromAny(targetObj["id"]), target),
			stringFromAny(targetObj["serviceName"]),
			stringFromAny(targetObj["status"]),
			stringFromAny(targetObj["summary"]),
			stringFromAny(targetObj["severity"]),
			stringFromAny(targetObj["confidence"]),
			corootEvidenceRawRef(targetObj),
		)
	}
	if status := stringFromAny(root["targetStatus"]); status != "" {
		appendStatus(target, "", status, "", "", "", corootEvidenceRawRef(root))
	}
	status := stringFromAny(root["status"])
	if status != "" && !isCorootEvidenceErrorStatus(status) {
		appendStatus(target, "", status, stringFromAny(root["summary"]), stringFromAny(root["severity"]), "", corootEvidenceRawRef(root))
	}
	return out
}

func corootEvidenceDependencyEdges(root map[string]any, target string) []observability.DependencyEdge {
	var out []observability.DependencyEdge
	for _, item := range objectSlice(firstNonNil(root["dependency_edges"], root["dependencyEdges"], root["edges"], root["edgeEvidence"])) {
		out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, "", ""))
	}
	if graph := objectFromAny(root["evidenceGraph"]); len(graph) > 0 {
		for _, item := range objectSlice(firstNonNil(graph["edges"], graph["dependency_edges"])) {
			out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, "", ""))
		}
	}
	deps := root["dependencies"]
	switch typed := deps.(type) {
	case []any:
		for _, item := range objectSlice(typed) {
			out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, "", ""))
		}
	case []map[string]any:
		for _, item := range typed {
			out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, "", ""))
		}
	case map[string]any:
		for _, item := range objectSlice(firstNonNil(typed["upstream"], typed["upstreams"])) {
			to := firstNonBlank(stringFromAny(item["id"]), stringFromAny(item["name"]), stringFromAny(item["service"]))
			out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, target, to))
		}
		for _, item := range objectSlice(firstNonNil(typed["downstream"], typed["downstreams"])) {
			from := firstNonBlank(stringFromAny(item["id"]), stringFromAny(item["name"]), stringFromAny(item["service"]))
			out = appendCorootEvidenceEdge(out, corootEvidenceEdgeFromObject(item, from, target))
		}
	}
	return out
}

func corootEvidenceEdgeFromObject(obj map[string]any, fallbackFrom, fallbackTo string) observability.DependencyEdge {
	from := firstNonBlank(
		stringFromAny(obj["from"]),
		stringFromAny(obj["source"]),
		stringFromAny(obj["sourceName"]),
		fallbackFrom,
	)
	to := firstNonBlank(
		stringFromAny(obj["to"]),
		stringFromAny(obj["target"]),
		stringFromAny(obj["targetName"]),
		fallbackTo,
	)
	return observability.DependencyEdge{
		From:       from,
		To:         to,
		Direction:  stringFromAny(obj["direction"]),
		Status:     firstNonBlank(stringFromAny(obj["status"]), stringFromAny(obj["connectivity"])),
		Summary:    firstNonBlank(stringFromAny(obj["summary"]), stringFromAny(obj["connectivityMessage"]), strings.Join(stringSlice(obj["stats"]), "; ")),
		Severity:   stringFromAny(obj["severity"]),
		Confidence: stringFromAny(obj["confidence"]),
		RawRef:     corootEvidenceRawRef(obj),
	}
}

func appendCorootEvidenceEdge(edges []observability.DependencyEdge, edge observability.DependencyEdge) []observability.DependencyEdge {
	if edge.From == "" || edge.To == "" {
		return edges
	}
	for _, existing := range edges {
		if existing.From == edge.From && existing.To == edge.To {
			return edges
		}
	}
	return append(edges, edge)
}

func corootEvidenceHypotheses(root map[string]any) []observability.Hypothesis {
	var out []observability.Hypothesis
	for _, obj := range objectSlice(root["hypotheses"]) {
		entity := firstNonBlank(
			stringFromAny(obj["entity"]),
			stringFromAny(obj["suspectService"]),
			stringFromAny(obj["service"]),
			stringFromAny(obj["name"]),
		)
		summary := firstNonBlank(
			stringFromAny(obj["summary"]),
			stringFromAny(obj["title"]),
			stringFromAny(obj["rootCause"]),
			stringFromAny(obj["description"]),
		)
		if entity == "" && summary == "" {
			continue
		}
		out = append(out, observability.Hypothesis{
			Entity:     entity,
			Summary:    summary,
			Severity:   stringFromAny(obj["severity"]),
			Confidence: stringFromAny(obj["confidence"]),
			Evidence:   stringSlice(obj["evidence"]),
			RawRef:     corootEvidenceRawRef(obj),
		})
	}
	if report := objectFromAny(root["referenceRca"]); len(report) > 0 {
		entity := firstNonBlank(stringFromAny(report["service"]), stringFromAny(report["applicationId"]))
		summary := firstNonBlank(stringFromAny(report["rootCause"]), stringFromAny(report["summary"]))
		if summary != "" {
			out = append(out, observability.Hypothesis{
				Entity:     entity,
				Summary:    summary,
				Confidence: "reference",
				RawRef:     corootEvidenceRawRef(report),
			})
		}
	}
	return out
}

func corootEvidenceIncidents(root map[string]any) []observability.IncidentEvidence {
	var out []observability.IncidentEvidence
	for _, obj := range objectSlice(firstNonNil(root["recentIncidents"], root["incidents"])) {
		id := firstNonBlank(stringFromAny(obj["id"]), stringFromAny(obj["key"]))
		summary := firstNonBlank(stringFromAny(obj["description"]), stringFromAny(obj["summary"]), stringFromAny(obj["rootCause"]))
		if id == "" && summary == "" {
			continue
		}
		out = append(out, observability.IncidentEvidence{
			ID:         id,
			Entity:     firstNonBlank(stringFromAny(obj["applicationId"]), stringFromAny(obj["application"])),
			Name:       stringFromAny(obj["key"]),
			Status:     firstNonBlank(stringFromAny(obj["state"]), stringFromAny(obj["rcaStatus"])),
			Summary:    summary,
			Severity:   stringFromAny(obj["severity"]),
			Confidence: "observed",
			RawRef:     corootEvidenceRawRef(obj),
		})
	}
	return out
}

func corootEvidenceMetrics(root map[string]any, target string) []observability.MetricEvidence {
	var out []observability.MetricEvidence
	for _, obj := range objectSlice(firstNonNil(root["metricSummaries"], root["metrics"])) {
		name := stringFromAny(obj["name"])
		summary := firstNonBlank(stringFromAny(obj["summary"]), stringFromAny(obj["chartTitle"]), stringFromAny(obj["topic"]))
		if name == "" && summary == "" {
			continue
		}
		out = append(out, observability.MetricEvidence{
			Entity:     firstNonBlank(stringFromAny(obj["entity"]), stringFromAny(obj["service"]), target),
			Name:       name,
			Status:     stringFromAny(obj["status"]),
			Value:      stringFromAny(obj["value"]),
			Unit:       stringFromAny(obj["unit"]),
			Summary:    summary,
			Severity:   stringFromAny(obj["severity"]),
			Confidence: "observed",
			RawRef:     corootEvidenceRawRef(obj),
		})
	}
	return out
}

func corootEvidenceLogs(root map[string]any) []observability.LogEvidence {
	logSummary := objectFromAny(root["logSummary"])
	if len(logSummary) == 0 {
		logSummary = objectFromAny(root["logs"])
	}
	if len(logSummary) == 0 {
		return nil
	}
	summary := fmt.Sprintf("matched=%d error_like=%d total=%d",
		intFromAny(logSummary["matchedCount"]),
		intFromAny(logSummary["errorLikeCount"]),
		intFromAny(logSummary["totalCount"]),
	)
	return []observability.LogEvidence{{
		Name:       "coroot logs",
		Summary:    summary,
		Severity:   corootEvidenceSeverityFromStatus(logSummary["status"]),
		Count:      intFromAny(firstNonNil(logSummary["matchedCount"], logSummary["totalCount"])),
		Confidence: "observed",
		RawRef:     corootEvidenceRawRef(logSummary),
	}}
}

func corootEvidenceTraces(root map[string]any) []observability.TraceEvidence {
	traceSummary := objectFromAny(root["traceSummary"])
	if len(traceSummary) == 0 {
		traceSummary = objectFromAny(root["traces"])
	}
	if len(traceSummary) == 0 {
		return nil
	}
	summary := fmt.Sprintf("spans=%d error_spans=%d",
		intFromAny(traceSummary["spanCount"]),
		intFromAny(traceSummary["errorSpanCount"]),
	)
	return []observability.TraceEvidence{{
		Name:       "coroot traces",
		Status:     stringFromAny(traceSummary["status"]),
		Summary:    summary,
		Severity:   corootEvidenceSeverityFromStatus(traceSummary["status"]),
		Confidence: "observed",
		RawRef:     corootEvidenceRawRef(traceSummary),
	}}
}

func corootEvidenceDeployments(root map[string]any) []observability.DeploymentEvidence {
	var out []observability.DeploymentEvidence
	for _, obj := range objectSlice(firstNonNil(root["deploymentEvents"], root["deployments"])) {
		summary := firstNonBlank(stringFromAny(obj["summary"]), strings.Join(stringSlice(obj["summary"]), "; "))
		entity := firstNonBlank(stringFromAny(obj["applicationId"]), stringFromAny(obj["application"]))
		version := stringFromAny(obj["version"])
		if entity == "" && version == "" && summary == "" {
			continue
		}
		out = append(out, observability.DeploymentEvidence{
			Entity:     entity,
			Name:       firstNonBlank(stringFromAny(obj["name"]), stringFromAny(obj["application"])),
			Version:    version,
			Status:     stringFromAny(obj["status"]),
			Summary:    summary,
			Severity:   corootEvidenceSeverityFromStatus(obj["status"]),
			Confidence: "observed",
			RawRef:     corootEvidenceRawRef(obj),
		})
	}
	return out
}

func corootEvidenceMissing(root map[string]any) []string {
	var out []string
	appendValues := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			out = appendCorootUniqueString(out, value, 32)
		}
	}
	appendValues(stringSlice(firstNonNil(root["missing_evidence"], root["missingEvidence"]))...)
	if summary := objectFromAny(root["summary"]); len(summary) > 0 {
		appendValues(stringSlice(firstNonNil(summary["missingEvidence"], summary["missing_evidence"]))...)
	}
	appendValues(stringSlice(root["limitations"])...)
	status := stringFromAny(root["status"])
	if isCorootEvidenceErrorStatus(status) {
		appendValues("Coroot evidence is unavailable: status=" + status)
	}
	if errObj := objectFromAny(root["error"]); len(errObj) > 0 {
		message := firstNonBlank(stringFromAny(errObj["message"]), stringFromAny(errObj["kind"]), "Coroot error")
		appendValues("Coroot evidence is unavailable: " + message)
	}
	sort.Strings(out)
	return out
}

func corootEvidenceRawRef(obj map[string]any) string {
	if len(obj) == 0 {
		return ""
	}
	raw := obj["rawRef"]
	if raw == nil {
		raw = obj["raw_ref"]
	}
	if text := stringFromAny(raw); text != "" {
		return text
	}
	rawObj := objectFromAny(raw)
	return firstNonBlank(stringFromAny(rawObj["uri"]), stringFromAny(rawObj["digest"]))
}

func corootEvidenceSeverityFromStatus(value any) string {
	status := strings.ToLower(strings.TrimSpace(stringFromAny(value)))
	switch status {
	case "critical", "warning", "error", "degraded":
		return status
	default:
		return ""
	}
}

func isCorootEvidenceErrorStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "unavailable", "not_configured", "timeout":
		return true
	default:
		return false
	}
}
