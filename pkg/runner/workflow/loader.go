package workflow

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadFile(path string) (Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workflow{}, err
	}
	return Load(data)
}

func Load(data []byte) (Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return Workflow{}, err
	}
	return wf, nil
}
