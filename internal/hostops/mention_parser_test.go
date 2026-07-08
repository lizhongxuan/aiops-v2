package hostops

import "testing"

func TestParseHostMentionsChineseConnector(t *testing.T) {
	input := "@1.1.1.1和@1.1.1.2作为目标主机执行通用运维任务,@1.1.1.3作为验证节点."
	mentions := ParseHostMentions(input)
	if len(mentions) != 3 {
		t.Fatalf("len(mentions) = %d, want 3: %#v", len(mentions), mentions)
	}
	if mentions[0].Raw != "@1.1.1.1" || mentions[1].Raw != "@1.1.1.2" || mentions[2].Raw != "@1.1.1.3" {
		t.Fatalf("mentions = %#v, want ordered IP mentions", mentions)
	}
}

func TestParseHostMentionsDedupesNormalizedToken(t *testing.T) {
	mentions := ParseHostMentions("@host-1 检查一下 @host-1 的状态")
	unique := UniqueMentionKeys(mentions)
	if len(unique) != 1 {
		t.Fatalf("len(unique) = %d, want 1: %#v", len(unique), unique)
	}
}

func TestParseHostMentionsIgnoresPlainEmail(t *testing.T) {
	mentions := ParseHostMentions("联系 sre@example.com，不要把邮箱解析成主机")
	if len(mentions) != 0 {
		t.Fatalf("mentions = %#v, want none", mentions)
	}
}

func TestParseHostMentionsIncludesLocalAlias(t *testing.T) {
	mentions := ParseHostMentions("@local 帮我只读检查主机状态")
	if len(mentions) != 1 {
		t.Fatalf("len(mentions) = %d, want 1: %#v", len(mentions), mentions)
	}
	if mentions[0].Raw != "@local" || mentions[0].Source != HostMentionSourceLocalAlias {
		t.Fatalf("mention = %#v, want local alias", mentions[0])
	}
}

func TestParseHostMentionsIncludesExplicitLocalAliases(t *testing.T) {
	for _, input := range []string{
		"@local 查看 CPU",
		"@server-local 查看 CPU",
		"@localhost 查看 CPU",
		"@127.0.0.1 查看 CPU",
	} {
		mentions := ParseHostMentions(input)
		if len(mentions) != 1 || mentions[0].Source != HostMentionSourceLocalAlias {
			t.Fatalf("%q parsed as %#v, want one local_alias mention", input, mentions)
		}
	}
}

func TestParseHostMentionsSkipsCorootObservabilityMention(t *testing.T) {
	mentions := ParseHostMentions("@Coroot 分析 order-api 延迟")
	if len(mentions) != 0 {
		t.Fatalf("mentions = %#v, want @Coroot to stay out of host mentions", mentions)
	}
}

func TestDetectInventoryHostMentionsDoesNotBindBareHostID(t *testing.T) {
	mentions := DetectInventoryHostMentions("在 host-a 上只读检查 CPU、内存和磁盘空间", []HostRecordView{
		{ID: "host-a", Hostname: "db-a", Address: "10.0.0.11", DisplayName: "DB A", Executable: true},
	})
	if len(mentions) != 0 {
		t.Fatalf("mentions = %#v, want none: host execution requires @host, @ip, or selected host context", mentions)
	}
}

func TestDetectInventoryHostMentionsSkipsBareServerLocal(t *testing.T) {
	mentions := DetectInventoryHostMentions("没有 host id 时不能默认 server-local", []HostRecordView{
		{ID: "server-local", Hostname: "localhost", Address: "server-local", DisplayName: "server-local", Executable: true},
	})
	if len(mentions) != 0 {
		t.Fatalf("mentions = %#v, want no bare server-local mention", mentions)
	}
}

func TestResourceBindingProjectionFromMention(t *testing.T) {
	mention := HostMention{
		Raw:         "@db-a",
		HostID:      "host-a",
		DisplayName: "db-a",
		Source:      HostMentionSourceInventory,
		Resolved:    true,
		Confidence:  1,
	}

	projection := ResourceBindingProjectionFromMention(mention)
	if projection.HostID != "host-a" || projection.Source != string(HostMentionSourceInventory) || !projection.Resolved {
		t.Fatalf("projection = %+v, want resolved host-a", projection)
	}
	ref := ResourceRefFromHostMention(mention)
	if ref.Type != "host" || ref.ID != "host-a" {
		t.Fatalf("resource ref = %+v, want host-a", ref)
	}
}
