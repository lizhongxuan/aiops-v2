package api

import (
	"io/fs"
	"os"
)

func osDirFSImpl(root string) fs.FS {
	return os.DirFS(root)
}
