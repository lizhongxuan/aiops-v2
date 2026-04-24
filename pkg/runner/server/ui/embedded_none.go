//go:build !runnerwebembed

package ui

import "io/fs"

func EmbeddedFS() (fs.FS, bool) {
	return nil, false
}
