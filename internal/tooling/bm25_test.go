package tooling

import "testing"

func TestBM25RanksTermFrequencyAndLengthNormalizedDocument(t *testing.T) {
	index := NewBM25Index([]BM25Document{
		{ID: 1, Text: "backup status overview"},
		{ID: 2, Text: "stanza repo path backup restore stanza repo path"},
	})

	results := index.Search("stanza repo path", 10)
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if results[0].ID != 2 {
		t.Fatalf("top result ID = %d, want schema-specific document 2; results=%+v", results[0].ID, results)
	}
	if results[0].Score <= 0 {
		t.Fatalf("top score = %f, want positive BM25 score", results[0].Score)
	}
}

func TestBM25ReturnsStableIDOrderForEqualScores(t *testing.T) {
	index := NewBM25Index([]BM25Document{
		{ID: 20, Text: "service metrics"},
		{ID: 10, Text: "service metrics"},
	})

	results := index.Search("service metrics", 10)
	if len(results) != 2 {
		t.Fatalf("Search() returned %d results, want 2", len(results))
	}
	if results[0].ID != 10 || results[1].ID != 20 {
		t.Fatalf("results order = %+v, want stable ascending ID order for equal scores", results)
	}
}
