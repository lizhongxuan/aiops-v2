package experiencepack

import "testing"

func TestCanonicalHashIgnoresFieldOrderAndAssetID(t *testing.T) {
	left := map[string]any{"b": 2, "a": 1, "asset_id": "sha256:bad"}
	right := map[string]any{"a": 1, "b": 2}
	leftID, err := HashCanonicalJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	rightID, err := HashCanonicalJSON(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftID != rightID {
		t.Fatalf("hash should ignore order and asset_id: %s != %s", leftID, rightID)
	}
}

func TestHashChangesWithContent(t *testing.T) {
	first, _ := HashCanonicalJSON(map[string]any{"a": 1})
	second, _ := HashCanonicalJSON(map[string]any{"a": 2})
	if first == second {
		t.Fatal("hash should change when content changes")
	}
}

func TestMarkdownNormalizeStable(t *testing.T) {
	first := HashSkillMarkdown("# Skill\r\n\nstep  \n")
	second := HashSkillMarkdown(" # Skill\n\nstep\n\n")
	if first != second {
		t.Fatalf("markdown hash should be stable after normalization: %s != %s", first, second)
	}
}
