package structure

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
)

// mask out file type bits for permission comparisons (e.g., ignore directory and symlink bits).
const permissionMask = 0o777 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

const (
	maxRetries   = 3
	retryBackoff = 1 * time.Second
)

// layerStream returns a reader over the flattened, uncompressed filesystem of
// the image. Single-layer images are streamed directly. Multi-layer images go
// through mutate.Extract, which applies whiteouts.
func layerStream(i v1.Image) (io.ReadCloser, error) {
	ls, err := i.Layers()
	if err != nil {
		return nil, fmt.Errorf("getting image layers: %w", err)
	}
	if len(ls) == 1 {
		rc, err := ls[0].Uncompressed()
		if err != nil {
			return nil, fmt.Errorf("getting uncompressed layer: %w", err)
		}
		return rc, nil
	}
	return mutate.Extract(i), nil
}

// indexImage streams the image filesystem once and builds an in-memory index
// of it, capturing the contents of the given paths. Nothing is written to disk.
// It retries on errors like unexpected EOF.
func indexImage(i v1.Image, capture map[string]bool) (*tarIndex, error) {
	reopen := func() (io.ReadCloser, error) { return layerStream(i) }

	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			time.Sleep(retryBackoff * time.Duration(attempt))
		}

		idx, err := tryIndexImage(reopen, capture)
		if err == nil {
			return idx, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("after %d attempts: %w", maxRetries, lastErr)
}

func tryIndexImage(reopen func() (io.ReadCloser, error), capture map[string]bool) (*tarIndex, error) {
	rc, err := reopen()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	idx, err := newTarIndex(rc, capture, reopen)
	if err != nil {
		return nil, fmt.Errorf("indexing image filesystem: %w", err)
	}
	return idx, nil
}

type Condition interface {
	Check(v1.Image, fs.FS) error
}

// contentCondition is implemented by conditions that need to read file
// contents, so the contents can be captured while the image is indexed.
type contentCondition interface {
	ContentPaths() []string
}

type Conditions []Condition

func (c Conditions) Check(i v1.Image) error {
	// Check if any condition needs the filesystem, and which file contents
	// have to be captured while indexing it.
	needsFS := false
	capture := map[string]bool{}
	for _, cond := range c {
		if _, ok := cond.(EnvCondition); !ok {
			needsFS = true
		}
		if cc, ok := cond.(contentCondition); ok {
			for _, p := range cc.ContentPaths() {
				capture[normalize(p)] = true
			}
		}
	}

	var fsys fs.FS
	if needsFS {
		idx, err := indexImage(i, capture)
		if err != nil {
			return fmt.Errorf("extracting layers: %w", err)
		}
		fsys = idx
	}

	var errs []error
	for _, cond := range c {
		errs = append(errs, cond.Check(i, fsys))
	}
	return errors.Join(errs...)
}

type EnvCondition struct {
	Want map[string]string
}

func (e EnvCondition) Check(i v1.Image, _ fs.FS) error {
	cf, err := i.ConfigFile()
	if err != nil {
		return fmt.Errorf("getting image config: %w", err)
	}
	var errs []error
	split := splitEnvs(cf.Config.Env)
	for k, v := range e.Want {
		if split[k] != v {
			errs = append(errs, fmt.Errorf("env %q does not match %q (got %q)", k, v, split[k]))
		}
		if separator, exists := verifyEnv[k]; exists {
			for p := range strings.SplitSeq(v, separator) {
				if !strings.HasPrefix(p, "/") || p == fmt.Sprintf("$%s", k) {
					errs = append(errs, fmt.Errorf("env %q value %q references relative path or literal $ string %q", k, v, p))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func splitEnvs(in []string) map[string]string {
	out := make(map[string]string, len(in))
	for _, i := range in {
		k, v, _ := strings.Cut(i, "=")
		out[k] = v
	}
	return out
}

type FilesCondition struct {
	Want map[string]File
}

type File struct {
	Optional bool
	Mode     *os.FileMode
	Regex    string
}

// ContentPaths returns the files whose contents are matched against a regex.
func (f FilesCondition) ContentPaths() []string {
	var paths []string
	for p, f := range f.Want {
		if f.Regex != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

func (f FilesCondition) Check(_ v1.Image, fsys fs.FS) error {
	var errs []error

	for path, f := range f.Want {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(path, "/")

		tf, err := fsys.Open(name)
		if err != nil {
			// Optional files may only exist across a subset
			// of structure test runs but we want to avoid erroring so that
			// we can specify these conditions without having to add per-image conditions
			if errors.Is(err, fs.ErrNotExist) && f.Optional {
				continue
			} else if errors.Is(err, fs.ErrNotExist) {
				// Avoid breaking backward compatibility.
				errs = append(errs, fmt.Errorf("file %q not found", path))
			} else {
				// Any other error is unexpected, so we want to retain it.
				errs = append(errs, fmt.Errorf("opening %q: %w", path, err))
			}
			continue
		}
		if f.Regex != "" {
			// We care about the contents, so read and buffer them and regexp.
			got, err := io.ReadAll(tf)
			if err != nil {
				errs = append(errs, fmt.Errorf("reading %q: %w", path, err))
				continue
			}

			if !regexp.MustCompile(f.Regex).Match(got) {
				errs = append(errs, fmt.Errorf("file %q does not match regexp %q, got:\n%s", path, f.Regex, got))
			}
		}
		if f.Mode != nil {
			stat, err := tf.Stat()
			if err != nil {
				errs = append(errs, fmt.Errorf("statting %q: %w", path, err))
				continue
			}

			got := stat.Mode() & permissionMask
			want := *f.Mode & permissionMask

			if got != want {
				errs = append(errs, fmt.Errorf("file %q mode does not match %o (got %o)", path, want, got))
			}
		}
	}

	return errors.Join(errs...)
}

type DirsCondition struct {
	Want map[string]Dir
}

type Dir struct {
	FilesOnly bool // only check file permissions within the directory [structure]
	Mode      *os.FileMode
	Recursive bool
}

func (d DirsCondition) Check(_ v1.Image, fsys fs.FS) error {
	var errs []error

	for path, dir := range d.Want {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(path, "/")

		if !dir.Recursive {
			fi, err := fs.Stat(fsys, name)
			if err != nil {
				errs = append(errs, fmt.Errorf("statting directory %q: %w", path, err))
			}
			got := fi.Mode() & permissionMask
			want := *dir.Mode & permissionMask

			// We only care about the single, top-level directory
			if fi.IsDir() && got != want {
				errs = append(errs, fmt.Errorf("directory %q mode does not match %o (got %o)", path, want, got))
			}
		} else {
			err := fs.WalkDir(fsys, name, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return fmt.Errorf("walking %q: %w", path, err)
				}

				// ignore symlinks which will register as 777
				if d.Type()&fs.ModeSymlink == fs.ModeSymlink {
					return nil
				}

				if dir.FilesOnly && d.IsDir() {
					return nil
				}

				fi, err := d.Info()
				if err != nil {
					errs = append(errs, fmt.Errorf("getting info for %q: %w", path, err))
				}

				got := fi.Mode() & permissionMask
				want := *dir.Mode & permissionMask

				if got != want {
					errs = append(errs, fmt.Errorf("file %q mode does not match %o (got %o)", path, want, got))
				}

				return nil
			})
			if err != nil {
				errs = append(errs, fmt.Errorf("walking directory %q: %w", path, err))
			}
		}
	}

	return errors.Join(errs...)
}

type PermissionsCondition struct {
	Want map[string]Permission
}

type Permission struct {
	Block     *os.FileMode
	Override  []string
	FilesOnly bool // only check regular files, skipping directories
}

func (p PermissionsCondition) Check(_ v1.Image, fsys fs.FS) error {
	var errs []error

	for path, perm := range p.Want {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(path, "/")

		err := fs.WalkDir(fsys, name, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("walking %q: %w", path, err)
			}

			// ignore symlinks which will register as 777
			if d.Type()&fs.ModeSymlink == fs.ModeSymlink {
				return nil
			}

			if perm.FilesOnly && d.IsDir() {
				return nil
			}

			fi, err := d.Info()
			if err != nil {
				errs = append(errs, fmt.Errorf("getting info for %q: %w", path, err))
			}

			got := fi.Mode() & permissionMask
			block := *perm.Block & permissionMask

			if got == block && !hasOverride(path, perm.Override) {
				errs = append(errs, fmt.Errorf("file %q mode matches blocked permission %o (got %o)", path, block, got))
			}

			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("walking directory %q: %w", path, err))
		}
	}

	return errors.Join(errs...)
}

func hasOverride(filePath string, overrides []string) bool {
	for _, override := range overrides {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(override, "/")
		if strings.HasPrefix(filePath, name) {
			if filePath == name || strings.HasPrefix(filePath, name+"/") {
				return true
			}
		}
	}
	return false
}
