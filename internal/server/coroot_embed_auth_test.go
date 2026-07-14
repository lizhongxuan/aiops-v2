package server

import (
	"testing"
	"time"
)

func TestCorootEmbedTrustHeadersAreSigned(t *testing.T) {
	headers := signedCorootEmbedHeaders(corootEmbedIdentity{
		User:   "lizhongxuan",
		Roles:  []string{"coroot-readonly"},
		Tenant: "default",
	}, "shared-secret", time.Unix(1720413600, 0))

	if headers.Get("X-Aiops-Embed-User") != "lizhongxuan" {
		t.Fatalf("missing user header")
	}
	if headers.Get("X-Aiops-Embed-Roles") != "coroot-readonly" {
		t.Fatalf("missing roles header")
	}
	if headers.Get("X-Aiops-Embed-Tenant") != "default" {
		t.Fatalf("missing tenant header")
	}
	if headers.Get("X-Aiops-Embed-Timestamp") == "" {
		t.Fatalf("missing timestamp header")
	}
	if headers.Get("X-Aiops-Embed-Signature") == "" {
		t.Fatalf("missing signature")
	}
}
