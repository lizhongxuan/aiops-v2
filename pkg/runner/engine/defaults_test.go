package engine

import "testing"

func TestDefaultRegistryIncludesHTTPAndBuiltinModules(t *testing.T) {
	reg := DefaultRegistry()
	for _, action := range []string{
		"http.request",
		"builtin.tcp_ping",
		"builtin.http_check",
		"builtin.ssl_expiry_check",
		"builtin.dns_resolve",
	} {
		if _, ok := reg.Get(action); !ok {
			t.Fatalf("default registry missing %s", action)
		}
	}
	if _, ok := reg.Get("builtin.icmp_ping"); ok {
		t.Fatalf("icmp placeholder should not be registered by default")
	}
}
