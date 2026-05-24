package envcontext

import (
	"strings"
	"testing"
	"time"
)

func TestApplyUserTurnDoesNotPromoteBareRedisQuestionToConfirmedLocalFocus(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "server-local",
		Input:     "帮我排查一下 redis 为什么连不上",
		Now:       fixedTime(),
	})

	if state.LastIntent != IntentAmbiguous {
		t.Fatalf("LastIntent = %q, want %q", state.LastIntent, IntentAmbiguous)
	}
	if state.CurrentFocus != nil {
		t.Fatalf("CurrentFocus = %#v, want nil for bare redis question", state.CurrentFocus)
	}

	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false, want an explicit no-focus section")
	}
	for _, want := range []string{"ContextIntent: ambiguous", "CurrentFocus: none", "No confirmed environment facts"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
	if strings.Contains(section.Content, "server-local redis") {
		t.Fatalf("section promoted default local redis:\n%s", section.Content)
	}
}

func TestApplyUserTurnCarriesConfirmedDockerRedisAcrossContinuation(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-a",
		Input:     "当前排查主机A上的 Docker Redis，容器 aiops-redis，镜像 redis:7-alpine，端口 36379",
		Now:       fixedTime(),
	})
	if state.CurrentFocus == nil {
		t.Fatal("CurrentFocus is nil, want confirmed docker redis focus")
	}
	if state.CurrentFocus.HostID != "host-a" || state.CurrentFocus.TargetKind != "redis" || state.CurrentFocus.DeploymentKind != DeploymentDocker {
		t.Fatalf("CurrentFocus = %#v, want host-a redis docker", state.CurrentFocus)
	}

	state = ApplyUserTurn(state, UserTurn{
		SessionID: "sess-1",
		Input:     "继续排查这个 Redis 的延迟问题",
		Now:       fixedTime().Add(time.Minute),
	})
	if state.LastIntent != IntentContinue {
		t.Fatalf("LastIntent = %q, want %q", state.LastIntent, IntentContinue)
	}
	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	for _, want := range []string{
		"Runtime Environment Context",
		"ContextIntent: continue",
		"CurrentFocus:",
		"host=host-a",
		"target=redis",
		"deployment=docker",
		"aiops-redis",
		"redis:7-alpine",
	} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
}

func TestApplyUserTurnCorrectionSwitchesFocusAndDropsOldDockerFacts(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-a",
		Input:     "主机A Docker Redis，容器 aiops-redis，端口 36379",
		Now:       fixedTime(),
	})
	state = ApplyUserTurn(state, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-b",
		Input:     "不对，后面说的是主机B上二进制部署的 Redis，不是 Docker，端口 6379",
		Now:       fixedTime().Add(time.Minute),
	})

	if state.LastIntent != IntentCorrection {
		t.Fatalf("LastIntent = %q, want %q", state.LastIntent, IntentCorrection)
	}
	if state.CurrentFocus == nil {
		t.Fatal("CurrentFocus is nil, want host-b host_process redis focus")
	}
	if state.CurrentFocus.HostID != "host-b" || state.CurrentFocus.DeploymentKind != DeploymentHostProcess {
		t.Fatalf("CurrentFocus = %#v, want host-b host_process", state.CurrentFocus)
	}
	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	for _, want := range []string{"host=host-b", "deployment=host_process", "port=6379"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
	for _, forbidden := range []string{"host-a", "aiops-redis", "port=36379", "deployment=docker"} {
		if strings.Contains(section.Content, forbidden) {
			t.Fatalf("section leaked old focus fact %q:\n%s", forbidden, section.Content)
		}
	}
}

func TestApplyUserTurnStagingInquiryDoesNotConfirmSelectedLocalHost(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "server-local",
		Input:     "现在看 staging 环境的 Redis，只需要排查，不要沿用刚才本地环境。",
		Now:       fixedTime(),
	})

	if state.LastIntent != IntentNewInquiry {
		t.Fatalf("LastIntent = %q, want %q", state.LastIntent, IntentNewInquiry)
	}
	if state.CurrentFocus == nil {
		t.Fatal("CurrentFocus is nil, want staging redis focus without confirmed local host")
	}
	if state.CurrentFocus.Environment != "staging" || state.CurrentFocus.TargetKind != "redis" {
		t.Fatalf("CurrentFocus = %#v, want staging redis focus", state.CurrentFocus)
	}
	if state.CurrentFocus.HostID != "" {
		t.Fatalf("CurrentFocus.HostID = %q, want empty because staging host is unconfirmed", state.CurrentFocus.HostID)
	}

	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	for _, want := range []string{"ContextIntent: new_inquiry", "environment=staging", "target=redis"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
	for _, forbidden := range []string{"host=server-local", "deployment=docker", "container=aiops-env-redis-a"} {
		if strings.Contains(section.Content, forbidden) {
			t.Fatalf("section leaked local focus fact %q:\n%s", forbidden, section.Content)
		}
	}
}

func TestApplyUserTurnExactR05CorrectionDropsSelectedLocalDockerFacts(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "server-local",
		Input:     "检查本机 Docker Redis aiops-env-redis-a。",
		Now:       fixedTime(),
	})
	state = ApplyUserTurn(state, UserTurn{
		SessionID: "sess-1",
		HostID:    "server-local",
		Input:     "不对，现在要检查主机 B 二进制部署的 Redis。",
		Now:       fixedTime().Add(time.Minute),
	})

	if state.LastIntent != IntentCorrection {
		t.Fatalf("LastIntent = %q, want %q", state.LastIntent, IntentCorrection)
	}
	if state.CurrentFocus == nil {
		t.Fatal("CurrentFocus is nil, want host-b host_process redis focus")
	}
	if state.CurrentFocus.HostID != "host-b" || state.CurrentFocus.DeploymentKind != DeploymentHostProcess {
		t.Fatalf("CurrentFocus = %#v, want host-b host_process", state.CurrentFocus)
	}

	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	for _, want := range []string{"ContextIntent: correction", "host=host-b", "deployment=host_process"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
	for _, forbidden := range []string{"host=server-local", "deployment=docker", "aiops-env-redis-a"} {
		if strings.Contains(section.Content, forbidden) {
			t.Fatalf("section leaked old local docker fact %q:\n%s", forbidden, section.Content)
		}
	}
}

func TestBuildRuntimeEnvironmentSectionRedactsSensitiveFacts(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-a",
		Input:     "Redis 连接串是 redis://:secret-pass@10.0.0.1:6379/0，目标是主机A上的二进制部署 Redis",
		Now:       fixedTime(),
	})

	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	if strings.Contains(section.Content, "secret-pass") {
		t.Fatalf("section leaked secret:\n%s", section.Content)
	}
	for _, want := range []string{"redis://:[REDACTED]@10.0.0.1:6379/0", "deployment=host_process"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
}

func TestManualSearchContextBindsToCurrentFocusWithoutPromotingRequiredParams(t *testing.T) {
	state := ApplyUserTurn(State{}, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-a",
		Input:     "先按运维手册A排查 Docker Redis，required params 示例 container_name=aiops-redis",
		Now:       fixedTime(),
	})

	section, ok := BuildRuntimeEnvironmentSection(state)
	if !ok {
		t.Fatal("BuildRuntimeEnvironmentSection returned false")
	}
	for _, want := range []string{"ManualSearchContext:", "BoundFocusID:", "BindingStatus: pending", "manual=A"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q:\n%s", want, section.Content)
		}
	}
	if strings.Contains(section.Content, "required params") {
		t.Fatalf("section promoted manual required params into runtime context:\n%s", section.Content)
	}

	state = ApplyUserTurn(state, UserTurn{
		SessionID: "sess-1",
		HostID:    "host-a",
		Input:     "现在切到运维手册B，针对主机进程 Redis，不要沿用手册A",
		Now:       fixedTime().Add(time.Minute),
	})
	section, _ = BuildRuntimeEnvironmentSection(state)
	for _, want := range []string{"manual=B", "deployment=host_process"} {
		if !strings.Contains(section.Content, want) {
			t.Fatalf("section missing %q after manual switch:\n%s", want, section.Content)
		}
	}
	for _, forbidden := range []string{"manual=A", "required params", "container_name=aiops-redis"} {
		if strings.Contains(section.Content, forbidden) {
			t.Fatalf("section leaked old manual data %q:\n%s", forbidden, section.Content)
		}
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
}
