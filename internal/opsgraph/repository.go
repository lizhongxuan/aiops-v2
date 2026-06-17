package opsgraph

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Repository interface {
	Load(ctx context.Context) (GraphDocument, error)
	Save(ctx context.Context, graph GraphDocument) error
}

type FileRepository struct {
	path string
}

func NewFileRepository(path string) *FileRepository {
	return &FileRepository{path: path}
}

func (r *FileRepository) Load(ctx context.Context) (GraphDocument, error) {
	if err := ctx.Err(); err != nil {
		return GraphDocument{}, err
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return GraphDocument{SchemaVersion: ManualGraphSchemaVersion}, nil
	}
	if err != nil {
		return GraphDocument{}, err
	}
	var doc GraphDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return GraphDocument{}, err
	}
	return doc.Normalized(), nil
}

func (r *FileRepository) Save(ctx context.Context, doc GraphDocument) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	doc = doc.Normalized()
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}
