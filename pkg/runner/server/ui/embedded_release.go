//go:build runnerwebembed

package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist/**
var distFS embed.FS

func EmbeddedFS() (fs.FS, bool) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub, true
}
