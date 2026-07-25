package workflowstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"runner/workflow/edit"
)

const workflowNamespaceVersion = "v1"

// WorkflowNamespace maps a canonical WorkflowKey to a private, stable storage
// namespace. Callers should hold Lock while performing a multi-file operation.
type WorkflowNamespace struct {
	root string
	dir  string
}

// NewWorkflowNamespace creates a path mapping without touching the filesystem.
// The canonical JSON digest avoids delimiter and tenant-prefix collisions.
func NewWorkflowNamespace(root string, key edit.WorkflowKey) (*WorkflowNamespace, error) {
	if err := key.Validate(); err != nil {
		return nil, fmt.Errorf("workflow namespace key: %w", err)
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("workflow namespace root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workflow namespace root: %w", err)
	}
	canonical, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("encode workflow namespace key: %w", err)
	}
	digest := sha256.Sum256(canonical)
	dir := filepath.Join(absoluteRoot, workflowNamespaceVersion, hex.EncodeToString(digest[:]))
	return &WorkflowNamespace{root: absoluteRoot, dir: dir}, nil
}

// Dir returns the namespace directory. It does not create the directory.
func (n *WorkflowNamespace) Dir() string {
	if n == nil {
		return ""
	}
	return n.dir
}

// Path resolves a canonical, portable relative path inside the namespace.
func (n *WorkflowNamespace) Path(relative string) (string, error) {
	if n == nil || n.dir == "" {
		return "", fmt.Errorf("workflow namespace is not initialized")
	}
	if err := validateNamespaceRelativePath(relative); err != nil {
		return "", err
	}
	joined := filepath.Join(n.dir, filepath.FromSlash(relative))
	rel, err := filepath.Rel(n.dir, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workflow namespace path escapes namespace")
	}
	return joined, nil
}

// AtomicWriteFile durably replaces a file with a same-directory temp file.
// The file and its parent directory are synced before success is reported.
func (n *WorkflowNamespace) AtomicWriteFile(relative string, data []byte, perm fs.FileMode) (retErr error) {
	if perm.Perm() == 0 {
		return fmt.Errorf("atomic file permission must be non-zero")
	}
	target, err := n.Path(relative)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if err := n.ensureDirectory(parent); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(parent, ".atomic-*")
	if err != nil {
		return fmt.Errorf("create atomic temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	temporaryClosed := false
	defer func() {
		if !temporaryClosed {
			closeErr := temporary.Close()
			temporaryClosed = true
			if closeErr != nil && retErr == nil {
				retErr = fmt.Errorf("close atomic temporary file: %w", closeErr)
			}
		}
		if removeErr := os.Remove(temporaryName); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && retErr == nil {
			retErr = fmt.Errorf("remove atomic temporary file: %w", removeErr)
		}
	}()

	if err := temporary.Chmod(perm.Perm()); err != nil {
		return fmt.Errorf("chmod atomic temporary file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write atomic temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync atomic temporary file: %w", err)
	}
	closeErr := temporary.Close()
	temporaryClosed = true
	if closeErr != nil {
		return fmt.Errorf("close atomic temporary file: %w", closeErr)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("rename atomic file: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return fmt.Errorf("sync atomic parent directory: %w", err)
	}
	return nil
}

// Lock acquires the cross-process correctness lock for this WorkflowKey.
func (n *WorkflowNamespace) Lock(ctx context.Context) (*FileLock, error) {
	if ctx == nil {
		return nil, fmt.Errorf("workflow namespace lock context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := n.ensureDirectory(n.dir); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(n.dir, ".workflow.lock")
	if info, err := os.Lstat(lockPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workflow namespace lock file is a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect workflow namespace lock file: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workflow namespace lock: %w", err)
	}
	if err := acquirePlatformLock(ctx, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire workflow namespace lock: %w", err)
	}
	return &FileLock{file: file}, nil
}

// WithLock runs fn while holding the cross-process WorkflowKey lock.
func (n *WorkflowNamespace) WithLock(ctx context.Context, fn func(*WorkflowNamespace) error) (retErr error) {
	if fn == nil {
		return fmt.Errorf("workflow namespace lock callback is required")
	}
	lock, err := n.Lock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		retErr = errors.Join(retErr, lock.Close())
	}()
	return fn(n)
}

// FileLock is an idempotently-releasable cross-process lock.
type FileLock struct {
	file *os.File
	once sync.Once
	err  error
}

func (l *FileLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.file == nil {
			return
		}
		l.err = errors.Join(releasePlatformLock(l.file), l.file.Close())
	})
	return l.err
}

func (n *WorkflowNamespace) ensureDirectory(target string) error {
	if n == nil || n.root == "" || n.dir == "" {
		return fmt.Errorf("workflow namespace is not initialized")
	}
	if err := os.MkdirAll(n.root, 0o700); err != nil {
		return fmt.Errorf("create workflow namespace root: %w", err)
	}
	relative, err := filepath.Rel(n.root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("workflow namespace directory escapes root")
	}
	current := n.root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		switch {
		case statErr == nil && info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("workflow namespace directory %q is a symlink", current)
		case statErr == nil && !info.IsDir():
			return fmt.Errorf("workflow namespace directory %q is not a directory", current)
		case statErr == nil:
			continue
		case !errors.Is(statErr, os.ErrNotExist):
			return fmt.Errorf("inspect workflow namespace directory %q: %w", current, statErr)
		}
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create workflow namespace directory %q: %w", current, err)
		}
		createdInfo, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("verify workflow namespace directory %q: %w", current, err)
		}
		if createdInfo.Mode()&os.ModeSymlink != 0 || !createdInfo.IsDir() {
			return fmt.Errorf("workflow namespace directory %q was replaced while creating it", current)
		}
	}
	return nil
}

func validateNamespaceRelativePath(relative string) error {
	if relative == "" || strings.TrimSpace(relative) != relative || strings.ContainsAny(relative, "\\:\x00") || filepath.IsAbs(relative) || path.IsAbs(relative) {
		return fmt.Errorf("invalid workflow namespace relative path %q", relative)
	}
	clean := path.Clean(relative)
	if clean != relative || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid workflow namespace relative path %q", relative)
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid workflow namespace relative path %q", relative)
		}
		for _, char := range segment {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
				continue
			}
			return fmt.Errorf("invalid workflow namespace relative path %q", relative)
		}
	}
	return nil
}
