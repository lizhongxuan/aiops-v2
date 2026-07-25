// Package workflowfixtures loads the repository's shared Workflow Canvas
// fixtures for Runner integration tests.
package workflowfixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FixtureSchemaVersion = "aiops.workflow-canvas.fixture/v1"

type Fixture struct {
	SchemaVersion string          `json:"schemaVersion"`
	FixtureID     string          `json:"fixtureId"`
	Kind          string          `json:"kind"`
	Data          json.RawMessage `json:"data"`
}

type fixtureRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Catalog struct {
	root     string
	fixtures map[string]fixtureRef
}

func LoadCatalog(manifestPath string) (*Catalog, error) {
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Fixtures []fixtureRef `json:"fixtures"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("decode fixture catalog: %w", err)
	}
	catalog := &Catalog{root: filepath.Dir(manifestPath), fixtures: make(map[string]fixtureRef, len(manifest.Fixtures))}
	for _, entry := range manifest.Fixtures {
		if entry.ID == "" || entry.Kind == "" || entry.Path == "" {
			return nil, fmt.Errorf("fixture catalog entry requires id, kind, and path")
		}
		if _, exists := catalog.fixtures[entry.ID]; exists {
			return nil, fmt.Errorf("duplicate fixture %s", entry.ID)
		}
		if err := validateRelativePath(entry.Path); err != nil {
			return nil, fmt.Errorf("fixture %s: %w", entry.ID, err)
		}
		catalog.fixtures[entry.ID] = entry
	}
	return catalog, nil
}

func (catalog *Catalog) Load(id string) (Fixture, error) {
	entry, exists := catalog.fixtures[id]
	if !exists {
		return Fixture{}, fmt.Errorf("unknown workflow fixture %s", id)
	}
	payload, err := os.ReadFile(filepath.Join(catalog.root, filepath.FromSlash(entry.Path)))
	if err != nil {
		return Fixture{}, err
	}
	var fixture Fixture
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		return Fixture{}, fmt.Errorf("decode fixture %s: %w", id, err)
	}
	if fixture.SchemaVersion != FixtureSchemaVersion {
		return Fixture{}, fmt.Errorf("fixture %s uses unsupported schema %q", id, fixture.SchemaVersion)
	}
	if fixture.FixtureID != id {
		return Fixture{}, fmt.Errorf("fixture identity mismatch: catalog=%s document=%s", id, fixture.FixtureID)
	}
	if fixture.Kind != entry.Kind {
		return Fixture{}, fmt.Errorf("fixture %s kind mismatch: catalog=%s document=%s", id, entry.Kind, fixture.Kind)
	}
	if len(fixture.Data) == 0 || bytes.Equal(fixture.Data, []byte("null")) {
		return Fixture{}, fmt.Errorf("fixture %s has empty data", id)
	}
	return fixture, nil
}

func (fixture Fixture) DecodeData(target any) error {
	if err := json.Unmarshal(fixture.Data, target); err != nil {
		return fmt.Errorf("decode fixture %s data: %w", fixture.FixtureID, err)
	}
	return nil
}

func validateRelativePath(path string) error {
	if filepath.IsAbs(path) || strings.Contains(filepath.ToSlash(filepath.Clean(path)), "../") || filepath.Clean(path) == ".." {
		return fmt.Errorf("path must remain under the manifest directory")
	}
	return nil
}
