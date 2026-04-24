//go:build runnerwebembed

package ui

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedFSContainsBuiltFrontendIndex(t *testing.T) {
	t.Parallel()

	embedded, ok := EmbeddedFS()
	if !ok {
		t.Fatal("expected embedded frontend assets to be available")
	}

	indexHTML, err := fs.ReadFile(embedded, "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}

	content := string(indexHTML)
	if strings.Contains(content, "UI dist is not available yet") {
		t.Fatalf("embedded index should not be fallback placeholder: %s", content)
	}
	if !strings.Contains(content, "/assets/") {
		t.Fatalf("embedded index should reference built assets: %s", content)
	}
}
