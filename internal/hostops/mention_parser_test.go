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
