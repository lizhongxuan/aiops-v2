package envcontext

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/promptcompiler"
)

type Intent string

const (
	IntentAmbiguous  Intent = "ambiguous"
	IntentContinue   Intent = "continue"
	IntentSwitch     Intent = "switch"
	IntentCorrection Intent = "correction"
	IntentNewInquiry Intent = "new_inquiry"
)

type DeploymentKind string

const (
	DeploymentUnknown     DeploymentKind = ""
	DeploymentDocker      DeploymentKind = "docker"
	DeploymentHostProcess DeploymentKind = "host_process"
	DeploymentKubernetes  DeploymentKind = "kubernetes"
)

type Confidence string

const (
	ConfidenceCandidate Confidence = "candidate"
	ConfidenceConfirmed Confidence = "confirmed"
)

type State struct {
	SessionID       string               `json:"sessionId,omitempty"`
	LastIntent      Intent               `json:"lastIntent,omitempty"`
	CurrentFocus    *Focus               `json:"currentFocus,omitempty"`
	ManualContext   *ManualSearchContext `json:"manualSearchContext,omitempty"`
	RetiredFocusIDs []string             `json:"retiredFocusIds,omitempty"`
	UpdatedAt       time.Time            `json:"updatedAt,omitempty"`
}

type UserTurn struct {
	SessionID string            `json:"sessionId,omitempty"`
	HostID    string            `json:"hostId,omitempty"`
	Input     string            `json:"input,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Now       time.Time         `json:"now,omitempty"`
}

type Focus struct {
	ID             string         `json:"id,omitempty"`
	SessionID      string         `json:"sessionId,omitempty"`
	HostID         string         `json:"hostId,omitempty"`
	Environment    string         `json:"environment,omitempty"`
	TargetKind     string         `json:"targetKind,omitempty"`
	TargetID       string         `json:"targetId,omitempty"`
	DeploymentKind DeploymentKind `json:"deploymentKind,omitempty"`
	Container      string         `json:"container,omitempty"`
	Namespace      string         `json:"namespace,omitempty"`
	Pod            string         `json:"pod,omitempty"`
	Version        string         `json:"version,omitempty"`
	Port           string         `json:"port,omitempty"`
	DSN            string         `json:"dsn,omitempty"`
	Confidence     Confidence     `json:"confidence,omitempty"`
	Source         string         `json:"source,omitempty"`
	CreatedAt      time.Time      `json:"createdAt,omitempty"`
	UpdatedAt      time.Time      `json:"updatedAt,omitempty"`
}

type ManualSearchContext struct {
	ManualID       string         `json:"manualId,omitempty"`
	TargetKind     string         `json:"targetKind,omitempty"`
	DeploymentKind DeploymentKind `json:"deploymentKind,omitempty"`
	BoundFocusID   string         `json:"boundFocusId,omitempty"`
	BindingStatus  string         `json:"bindingStatus,omitempty"`
	UpdatedAt      time.Time      `json:"updatedAt,omitempty"`
}

type extractedFocus struct {
	Focus
	HasExplicitTarget bool
	HasExplicitScope  bool
}

func ApplyUserTurn(state State, turn UserTurn) State {
	now := turn.Now
	if now.IsZero() {
		now = time.Now()
	}
	state.SessionID = firstNonEmpty(turn.SessionID, state.SessionID)
	input := strings.TrimSpace(turn.Input)
	intent := inferIntent(input, state.CurrentFocus != nil)
	state.LastIntent = intent
	state.UpdatedAt = now

	parsed := extractFocus(input, turn.HostID, state.SessionID, now)
	hasConfirmedFocus := parsed.HasExplicitTarget && parsed.HasExplicitScope

	switch intent {
	case IntentCorrection, IntentSwitch, IntentNewInquiry:
		if state.CurrentFocus != nil {
			state.RetiredFocusIDs = appendIfMissing(state.RetiredFocusIDs, state.CurrentFocus.ID)
		}
		if hasConfirmedFocus {
			focus := parsed.Focus
			focus.Confidence = ConfidenceConfirmed
			state.CurrentFocus = &focus
		} else if intent != IntentContinue {
			state.CurrentFocus = nil
		}
	case IntentContinue:
		if hasConfirmedFocus {
			focus := parsed.Focus
			focus.Confidence = ConfidenceConfirmed
			state.CurrentFocus = &focus
		} else if state.CurrentFocus != nil {
			state.CurrentFocus.UpdatedAt = now
		}
	default:
		if hasConfirmedFocus {
			focus := parsed.Focus
			focus.Confidence = ConfidenceConfirmed
			state.CurrentFocus = &focus
			state.LastIntent = IntentSwitch
		}
	}

	if manual := extractManualContext(input, state.CurrentFocus, now); manual != nil {
		state.ManualContext = manual
	}
	return state
}

func BuildRuntimeEnvironmentSection(state State) (promptcompiler.PromptSection, bool) {
	intent := strings.TrimSpace(string(state.LastIntent))
	if intent == "" {
		intent = string(IntentAmbiguous)
	}
	lines := []string{
		"Runtime Environment Context (dynamic; confirmed facts only)",
		"ContextIntent: " + intent,
	}
	if state.CurrentFocus == nil || state.CurrentFocus.Confidence != ConfidenceConfirmed {
		lines = append(lines,
			"CurrentFocus: none",
			"No confirmed environment facts. Do not assume local Redis, Docker, host process, Kubernetes namespace, ports, containers, versions, or manuals unless confirmed by user/tool evidence.",
		)
	} else {
		lines = append(lines,
			"CurrentFocus: "+focusPromptLine(*state.CurrentFocus),
			"EnvironmentFacts:",
		)
		lines = append(lines, focusFactLines(*state.CurrentFocus)...)
	}
	if state.ManualContext != nil {
		lines = append(lines, manualPromptLines(*state.ManualContext)...)
	}
	return promptcompiler.PromptSection{
		Title:   "Runtime Environment Context",
		Content: diagnostics.RedactSensitiveText(strings.Join(lines, "\n")),
	}, true
}

func inferIntent(input string, hasFocus bool) Intent {
	lower := strings.ToLower(input)
	switch {
	case containsAny(lower, "不对", "不是", "纠正", "更正"):
		return IntentCorrection
	case containsAny(lower, "切到", "切换", "换成", "改为", "现在是", "现在改", "后面说的是"):
		return IntentSwitch
	case containsAny(lower, "不要沿用", "不要带入旧", "不要继承", "其他环境", "另一个环境"):
		return IntentNewInquiry
	case hasFocus && containsAny(lower, "继续", "这个", "刚才", "当前这个", "还是"):
		return IntentContinue
	default:
		return IntentAmbiguous
	}
}

func extractFocus(input, requestHostID, sessionID string, now time.Time) extractedFocus {
	lower := strings.ToLower(input)
	clean := dropManualExampleFragments(input)
	cleanLower := strings.ToLower(clean)
	focus := Focus{
		SessionID:  sessionID,
		HostID:     extractHostID(clean, requestHostID),
		TargetKind: extractTargetKind(cleanLower),
		Source:     "user_explicit",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	focus.Environment = extractEnvironment(cleanLower)
	focus.DeploymentKind = extractDeploymentKind(cleanLower)
	focus.Container = extractContainer(clean)
	focus.Namespace = extractAfterKey(clean, `(?i)(?:namespace|ns|命名空间)\s*[:=：]?\s*([a-zA-Z0-9._-]+)`)
	focus.Pod = extractAfterKey(clean, `(?i)(?:pod)\s*[:=：]?\s*([a-zA-Z0-9._-]+)`)
	focus.Version = extractVersion(clean)
	focus.Port = extractPort(clean)
	focus.DSN = extractDSN(clean)
	if focus.Environment != "" && !positiveHostMentioned(cleanLower) {
		focus.HostID = ""
	}
	focus.TargetID = buildTargetID(focus)
	focus.ID = focusID(focus)

	explicitTarget := focus.TargetKind != ""
	explicitScope := focus.DeploymentKind != DeploymentUnknown ||
		focus.Container != "" ||
		focus.Namespace != "" ||
		focus.Pod != "" ||
		focus.Port != "" ||
		focus.DSN != "" ||
		hostMentioned(lower) ||
		focus.Environment != ""
	return extractedFocus{Focus: focus, HasExplicitTarget: explicitTarget, HasExplicitScope: explicitScope}
}

func extractManualContext(input string, focus *Focus, now time.Time) *ManualSearchContext {
	if !containsAny(input, "运维手册", "手册") {
		return nil
	}
	manualID := extractManualID(input)
	targetKind := extractTargetKind(strings.ToLower(input))
	deployment := extractDeploymentKind(strings.ToLower(input))
	boundFocusID := ""
	if focus != nil {
		boundFocusID = focus.ID
		if targetKind == "" {
			targetKind = focus.TargetKind
		}
		if deployment == DeploymentUnknown {
			deployment = focus.DeploymentKind
		}
	}
	if manualID == "" {
		manualID = "unknown"
	}
	return &ManualSearchContext{
		ManualID:       manualID,
		TargetKind:     targetKind,
		DeploymentKind: deployment,
		BoundFocusID:   boundFocusID,
		BindingStatus:  "pending",
		UpdatedAt:      now,
	}
}

func focusPromptLine(f Focus) string {
	parts := []string{"id=" + f.ID}
	addKV(&parts, "host", f.HostID)
	addKV(&parts, "environment", f.Environment)
	addKV(&parts, "target", f.TargetKind)
	addKV(&parts, "deployment", string(f.DeploymentKind))
	addKV(&parts, "targetID", f.TargetID)
	addKV(&parts, "container", f.Container)
	addKV(&parts, "namespace", f.Namespace)
	addKV(&parts, "pod", f.Pod)
	addKV(&parts, "version", f.Version)
	addKV(&parts, "port", f.Port)
	addKV(&parts, "dsn", f.DSN)
	addKV(&parts, "confidence", string(f.Confidence))
	return strings.Join(parts, " ")
}

func focusFactLines(f Focus) []string {
	var lines []string
	addFact := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		lines = append(lines, fmt.Sprintf("- %s=%s source=user_explicit confidence=confirmed", key, value))
	}
	addFact("host", f.HostID)
	addFact("environment", f.Environment)
	addFact("target", f.TargetKind)
	addFact("deployment", string(f.DeploymentKind))
	addFact("targetID", f.TargetID)
	addFact("container", f.Container)
	addFact("namespace", f.Namespace)
	addFact("pod", f.Pod)
	addFact("version", f.Version)
	addFact("port", f.Port)
	addFact("dsn", f.DSN)
	if len(lines) == 0 {
		return []string{"- none"}
	}
	return lines
}

func manualPromptLines(manual ManualSearchContext) []string {
	bound := firstNonEmpty(manual.BoundFocusID, "none")
	target := firstNonEmpty(manual.TargetKind, "unknown")
	deployment := firstNonEmpty(string(manual.DeploymentKind), "unknown")
	return []string{
		fmt.Sprintf("ManualSearchContext: manual=%s target=%s deployment=%s", manual.ManualID, target, deployment),
		"BoundFocusID: " + bound,
		"BindingStatus: " + firstNonEmpty(manual.BindingStatus, "pending"),
		"Manual defaults and examples are search hints only; they are not active EnvironmentFacts until confirmed by user or tool evidence.",
	}
}

func extractHostID(input, requestHostID string) string {
	lower := strings.ToLower(input)
	switch {
	case containsAny(lower, "主机b", "主机 b", "host-b", "host b"):
		return "host-b"
	case containsAny(lower, "主机a", "主机 a", "host-a", "host a"):
		return "host-a"
	case localHostNegated(lower):
		return ""
	case containsAny(lower, "server-local", "本机", "当前主机", "本地"):
		return firstNonEmpty(requestHostID, "server-local")
	default:
		return strings.TrimSpace(requestHostID)
	}
}

func extractTargetKind(lower string) string {
	if strings.Contains(lower, "redis") {
		return "redis"
	}
	if strings.Contains(lower, "mysql") {
		return "mysql"
	}
	if strings.Contains(lower, "nginx") {
		return "nginx"
	}
	return ""
}

func extractDeploymentKind(lower string) DeploymentKind {
	switch {
	case containsAny(lower, "不是 docker", "不是docker", "非 docker", "非docker", "二进制", "主机进程", "宿主机原生", "host_process", "systemd"):
		return DeploymentHostProcess
	case containsAny(lower, "k8s", "kubernetes", "namespace", "命名空间", "pod"):
		return DeploymentKubernetes
	case containsAny(lower, "docker", "容器", "镜像"):
		return DeploymentDocker
	default:
		return DeploymentUnknown
	}
}

func extractEnvironment(lower string) string {
	switch {
	case strings.Contains(lower, "staging"):
		return "staging"
	case strings.Contains(lower, "prod") || strings.Contains(lower, "生产"):
		return "prod"
	case strings.Contains(lower, "测试环境"):
		return "test"
	default:
		return ""
	}
}

func extractContainer(input string) string {
	return extractAfterKey(input, `(?i)(?:容器|container)\s*[:=：]?\s*([a-zA-Z0-9._-]+)`)
}

func extractVersion(input string) string {
	if value := extractAfterKey(input, `(?i)(?:镜像|image)\s*[:=：]?\s*([a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+)`); value != "" {
		return value
	}
	return extractAfterKey(input, `(?i)\b(redis:[a-zA-Z0-9._-]+)\b`)
}

func extractPort(input string) string {
	if value := extractAfterKey(input, `(?i)(?:端口|port)\s*[:=：]?\s*([0-9]{2,5})`); value != "" {
		return value
	}
	return ""
}

func extractDSN(input string) string {
	return extractAfterKey(input, `(?i)\b(redis://[^\s，,；;]+)`)
}

func extractManualID(input string) string {
	if value := extractAfterKey(input, `(?:运维手册|手册)\s*([A-Za-z0-9_-]+)`); value != "" {
		return value
	}
	return ""
}

func extractAfterKey(input, pattern string) string {
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(input)
	if len(match) < 2 {
		return ""
	}
	return strings.Trim(match[1], " \t\r\n，,。；;）)")
}

func dropManualExampleFragments(input string) string {
	lower := strings.ToLower(input)
	for _, marker := range []string{"required params", "required_params", "示例", "默认值"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			return strings.TrimSpace(input[:idx])
		}
	}
	return input
}

func buildTargetID(f Focus) string {
	switch {
	case f.DeploymentKind == DeploymentDocker && f.Container != "":
		return "docker:" + f.Container
	case f.DeploymentKind == DeploymentKubernetes && f.Namespace != "" && f.Pod != "":
		return "k8s:" + f.Namespace + "/" + f.Pod
	case f.DSN != "":
		return "dsn:" + f.DSN
	case f.Port != "" && f.HostID != "":
		return f.HostID + ":" + f.Port
	case f.TargetKind != "":
		return f.TargetKind
	default:
		return ""
	}
}

func focusID(f Focus) string {
	parts := []string{f.SessionID, f.HostID, f.Environment, f.TargetKind, string(f.DeploymentKind), f.TargetID}
	digest := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "focus-" + hex.EncodeToString(digest[:])[:12]
}

func hostMentioned(input string) bool {
	return containsAny(input, "主机a", "主机 a", "host-a", "host a", "主机b", "主机 b", "host-b", "host b", "server-local")
}

func positiveHostMentioned(input string) bool {
	return containsAny(input, "主机a", "主机 a", "host-a", "host a", "主机b", "主机 b", "host-b", "host b", "server-local") ||
		(!localHostNegated(input) && containsAny(input, "本机", "当前主机", "本地"))
}

func localHostNegated(input string) bool {
	return containsAny(input, "不是本地", "非本地", "不要沿用", "不要继承", "不要带入旧", "其他环境", "另一个环境")
}

func containsAny(input string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(input, strings.ToLower(needle)) || strings.Contains(input, needle) {
			return true
		}
	}
	return false
}

func appendIfMissing(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func addKV(parts *[]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*parts = append(*parts, key+"="+value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
