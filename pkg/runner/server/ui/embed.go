package ui

import (
	"embed"
	"io/fs"
)

//go:embed fallback/index.html
var fallbackFS embed.FS

func FallbackFS() fs.FS {
	sub, err := fs.Sub(fallbackFS, "fallback")
	if err != nil {
		panic(err)
	}
	return sub
}
