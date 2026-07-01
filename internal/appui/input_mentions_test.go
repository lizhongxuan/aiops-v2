package appui

import "testing"

func TestParseInputMentionsValidHostAndCapability(t *testing.T) {
	raw := `{"version":1,"mentions":[` +
		`{"version":1,"tokenId":"mention-0-local","sigil":"@","display":"@local","rawText":"@local","kind":"host","path":"host://server-local","source":"selection","range":{"start":0,"end":6},"payload":{"hostId":"server-local","address":"server-local","displayName":"local"}},` +
		`{"version":1,"tokenId":"mention-8-coroot","sigil":"@","display":"@Coroot","rawText":"@Coroot","kind":"capability","path":"capability://coroot","source":"selection","range":{"start":8,"end":15}}` +
		`]}`
	parsed := parseInputMentions("@local  @Coroot 分析", map[string]string{metadataInputMentionsV1: raw})

	if !parsed.Present || parsed.Invalid {
		t.Fatalf("parsed = %+v, want present valid input mentions", parsed)
	}
	if len(parsed.Hosts) != 1 || parsed.Hosts[0].HostID != "server-local" {
		t.Fatalf("Hosts = %+v, want server-local host hint", parsed.Hosts)
	}
	if !parsed.HasCapability("coroot") {
		t.Fatalf("Capabilities = %+v, want coroot", parsed.Capabilities)
	}
	if parsed.Source != "structured" || parsed.Validation != "confirmed" {
		t.Fatalf("Source/Validation = %q/%q, want structured/confirmed", parsed.Source, parsed.Validation)
	}
}

func TestParseInputMentionsValidOpsResources(t *testing.T) {
	raw := `{"version":1,"mentions":[` +
		`{"version":1,"tokenId":"mention-0-manual","sigil":"@","display":"Redis 内存压力排障","rawText":"@manual-manual-redis-memory","kind":"ops_manual","path":"ops-manual://manual-redis-memory","source":"selection","range":{"start":0,"end":27},"payload":{"manualId":"manual-redis-memory","title":"Redis 内存压力排障","workflowId":"workflow-redis-memory"}},` +
		`{"version":1,"tokenId":"mention-28-graph","sigil":"@","display":"生产服务图谱","rawText":"@opsgraph-graph.prod","kind":"ops_graph","path":"ops-graph://graph.prod","source":"selection","range":{"start":28,"end":48},"payload":{"graphId":"graph.prod","name":"生产服务图谱"}}` +
		`]}`
	parsed := parseInputMentions("@manual-manual-redis-memory @opsgraph-graph.prod 分析", map[string]string{metadataInputMentionsV1: raw})

	if !parsed.Present || parsed.Invalid {
		t.Fatalf("parsed = %+v, want present valid resource mentions", parsed)
	}
	if !parsed.HasCapability("ops_manuals") || !parsed.HasCapability("ops_graph") {
		t.Fatalf("Capabilities = %+v, want ops manuals and ops graph capabilities", parsed.Capabilities)
	}
	if len(parsed.Resources) != 2 {
		t.Fatalf("Resources = %+v, want two resource hints", parsed.Resources)
	}
	if parsed.Resources[0].Kind != "ops_manual" || parsed.Resources[0].ID != "manual-redis-memory" {
		t.Fatalf("Resources[0] = %+v, want selected ops manual", parsed.Resources[0])
	}
	if parsed.Resources[1].Kind != "ops_graph" || parsed.Resources[1].ID != "graph.prod" {
		t.Fatalf("Resources[1] = %+v, want selected ops graph", parsed.Resources[1])
	}
}

func TestParseInputMentionsRejectsStaleRange(t *testing.T) {
	raw := `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-local","sigil":"@","display":"@local","rawText":"@local","kind":"host","path":"host://server-local","source":"selection","range":{"start":0,"end":6},"payload":{"hostId":"server-local"}}]}`
	parsed := parseInputMentions("@hostA 查看 CPU", map[string]string{metadataInputMentionsV1: raw})

	if !parsed.Present || !parsed.Invalid {
		t.Fatalf("parsed = %+v, want present invalid stale mention", parsed)
	}
	if len(parsed.Hosts) != 0 {
		t.Fatalf("Hosts = %+v, want stale host dropped", parsed.Hosts)
	}
	if parsed.Validation != "invalid" {
		t.Fatalf("Validation = %q, want invalid", parsed.Validation)
	}
}

func TestParseInputMentionsRejectsUnknownPathScheme(t *testing.T) {
	raw := `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-x","sigil":"@","display":"@x","rawText":"@x","kind":"host","path":"ssh://host-a","source":"selection","range":{"start":0,"end":2},"payload":{"hostId":"host-a"}}]}`
	parsed := parseInputMentions("@x 检查", map[string]string{metadataInputMentionsV1: raw})

	if !parsed.Present || !parsed.Invalid {
		t.Fatalf("parsed = %+v, want invalid unknown path scheme", parsed)
	}
	if len(parsed.Hosts) != 0 || len(parsed.Capabilities) != 0 {
		t.Fatalf("parsed = %+v, want no strong mentions", parsed)
	}
}

func TestParseInputMentionsKeepsTypedFallbackWeak(t *testing.T) {
	raw := `{"version":1,"mentions":[` +
		`{"version":1,"tokenId":"mention-0-local","sigil":"@","display":"@local","rawText":"@local","kind":"host","path":"host://server-local","source":"typed_fallback","range":{"start":0,"end":6},"payload":{"hostId":"server-local"}},` +
		`{"version":1,"tokenId":"mention-8-coroot","sigil":"@","display":"@Coroot","rawText":"@Coroot","kind":"capability","path":"capability://coroot","source":"typed_fallback","range":{"start":8,"end":15}}` +
		`]}`
	parsed := parseInputMentions("@local  @Coroot 分析", map[string]string{metadataInputMentionsV1: raw})

	if !parsed.Present || parsed.Invalid {
		t.Fatalf("parsed = %+v, want present valid weak mentions", parsed)
	}
	if len(parsed.Hosts) != 0 || len(parsed.Capabilities) != 0 {
		t.Fatalf("parsed = %+v, want typed fallback to stay weak", parsed)
	}
}

func TestParseInputMentionsAbsentWhenMetadataMissing(t *testing.T) {
	parsed := parseInputMentions("@local 检查", nil)

	if parsed.Present || parsed.Invalid {
		t.Fatalf("parsed = %+v, want absent valid empty result", parsed)
	}
	if parsed.Source != "absent" || parsed.Validation != "absent" {
		t.Fatalf("Source/Validation = %q/%q, want absent/absent", parsed.Source, parsed.Validation)
	}
}
