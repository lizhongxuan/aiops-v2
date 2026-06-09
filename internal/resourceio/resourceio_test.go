package resourceio_test

import (
	"strings"
	"testing"

	"aiops-v2/internal/resourceio"
)

func TestReadTextRangeUsesOffsetLimit(t *testing.T) {
	result := resourceio.ReadText("alpha\nbeta\ngamma\n", resourceio.ReadRequest{Offset: 6, Limit: 4})

	if result.Content != "beta" {
		t.Fatalf("content = %q, want %q", result.Content, "beta")
	}
	if result.Offset != 6 {
		t.Fatalf("offset = %d, want 6", result.Offset)
	}
	if result.Limit != 4 {
		t.Fatalf("limit = %d, want 4", result.Limit)
	}
	if !result.Truncated {
		t.Fatalf("truncated = false, want true")
	}
}

func TestReadTextRangeUsesPageWhenOffsetMissing(t *testing.T) {
	result := resourceio.ReadText("0123456789abcdef", resourceio.ReadRequest{Limit: 4, Page: 2})

	if result.Content != "4567" {
		t.Fatalf("content = %q, want %q", result.Content, "4567")
	}
	if result.Offset != 4 {
		t.Fatalf("offset = %d, want 4", result.Offset)
	}
	if result.Page != 2 {
		t.Fatalf("page = %d, want 2", result.Page)
	}
}

func TestReadTextRangeClampsLimit(t *testing.T) {
	content := strings.Repeat("x", resourceio.DefaultMaxReadBytes+32)

	result := resourceio.ReadText(content, resourceio.ReadRequest{Limit: resourceio.DefaultMaxReadBytes + 1})

	if len(result.Content) != resourceio.DefaultMaxReadBytes {
		t.Fatalf("content length = %d, want %d", len(result.Content), resourceio.DefaultMaxReadBytes)
	}
	if result.Limit != resourceio.DefaultMaxReadBytes {
		t.Fatalf("limit = %d, want %d", result.Limit, resourceio.DefaultMaxReadBytes)
	}
	if !result.Truncated {
		t.Fatalf("truncated = false, want true")
	}
}

func TestReadTextQueryReturnsBoundedMatches(t *testing.T) {
	result := resourceio.ReadText("prefix target suffix", resourceio.ReadRequest{Query: "target", Limit: 12})

	if len(result.Matches) != 1 {
		t.Fatalf("matches length = %d, want 1", len(result.Matches))
	}
	if !strings.Contains(result.Matches[0].Content, "target") {
		t.Fatalf("match content = %q, want content containing target", result.Matches[0].Content)
	}
	if len(result.Matches[0].Content) > 12 {
		t.Fatalf("match content length = %d, want <= 12", len(result.Matches[0].Content))
	}
	if result.Content != "" {
		t.Fatalf("content = %q, want empty content for query result", result.Content)
	}
}

func TestReadTextQueryIsCaseInsensitive(t *testing.T) {
	result := resourceio.ReadText("prefix target suffix", resourceio.ReadRequest{Query: "TARGET", Limit: 12})

	if len(result.Matches) != 1 {
		t.Fatalf("matches length = %d, want 1", len(result.Matches))
	}
	if !strings.Contains(result.Matches[0].Content, "target") {
		t.Fatalf("match content = %q, want content containing target", result.Matches[0].Content)
	}
}

func TestReadTextQueryWindowNeverExceedsLimit(t *testing.T) {
	result := resourceio.ReadText("prefix very-long-target suffix", resourceio.ReadRequest{Query: "very-long-target", Limit: 6})

	if len(result.Matches) != 1 {
		t.Fatalf("matches length = %d, want 1", len(result.Matches))
	}
	if len(result.Matches[0].Content) > 6 {
		t.Fatalf("match content length = %d, want <= 6: %q", len(result.Matches[0].Content), result.Matches[0].Content)
	}
}

func TestReadTextMetadataOnlyReturnsNoContentOrMatches(t *testing.T) {
	result := resourceio.ReadText("prefix target suffix", resourceio.ReadRequest{Query: "target", Format: string(resourceio.FormatMetadata)})

	if !result.MetadataOnly {
		t.Fatalf("metadataOnly = false, want true")
	}
	if result.Content != "" {
		t.Fatalf("content = %q, want empty", result.Content)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("matches length = %d, want 0", len(result.Matches))
	}
}

func TestDigestContentIsStable(t *testing.T) {
	first := resourceio.DigestContent([]byte("stable"))
	second := resourceio.DigestContent([]byte("stable"))

	if first == "" {
		t.Fatalf("digest is empty")
	}
	if first != second {
		t.Fatalf("digest is not stable: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("digest = %q, want sha256 prefix", first)
	}
}

func TestDetectContentTypeTreatsJSONTextAsApplicationJSON(t *testing.T) {
	contentType := resourceio.DetectContentType("resource.json", []byte(`{"ok":true}`), "")

	if contentType != "application/json" {
		t.Fatalf("contentType = %q, want application/json", contentType)
	}
}

func TestBinaryReadReturnsMetadataOnly(t *testing.T) {
	result := resourceio.ReadBytes([]byte{0x00, 0x01, 0x02}, resourceio.ReadRequest{URI: "resource://binary", Limit: 10, Format: string(resourceio.FormatText)}, "application/octet-stream")

	if !result.MetadataOnly {
		t.Fatalf("metadataOnly = false, want true")
	}
	if result.Content != "" {
		t.Fatalf("content = %q, want empty", result.Content)
	}
	if result.Ref.Digest == "" {
		t.Fatalf("digest is empty")
	}
	if result.Ref.Bytes != 3 {
		t.Fatalf("bytes = %d, want 3", result.Ref.Bytes)
	}
	if result.Ref.ContentType != "application/octet-stream" {
		t.Fatalf("contentType = %q, want application/octet-stream", result.Ref.ContentType)
	}
}
