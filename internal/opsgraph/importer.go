package opsgraph

import (
	"os"

	"gopkg.in/yaml.v3"
)

type seedFile struct {
	Entities      []Entity       `yaml:"entities"`
	Relationships []Relationship `yaml:"relationships"`
}

func LoadSeedFile(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadSeed(data)
}

func LoadSeed(data []byte) (*Store, error) {
	var seed seedFile
	if err := yaml.Unmarshal(data, &seed); err != nil {
		return nil, err
	}
	return NewStore(seed.Entities, seed.Relationships), nil
}
