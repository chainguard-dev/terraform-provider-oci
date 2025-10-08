package structure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/jonjohnsonjr/targz/tarfs"
)

// mask out file type bits for permission comparisons (e.g., ignore directory and symlink bits).
const permissionMask = 0o777 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

type Condition interface {
	Check(v1.Image) error
}

type Conditions []Condition

func (c Conditions) Check(i v1.Image) error {
	var errs []error
	for _, cond := range c {
		errs = append(errs, cond.Check(i))
	}
	return errors.Join(errs...)
}

type EnvCondition struct {
	Want map[string]string
}

func (e EnvCondition) Check(i v1.Image) error {
	cf, err := i.ConfigFile()
	if err != nil {
		return err
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

const maxRegexFileSize = 1 * 1024 * 1024 // 1MB

func (f FilesCondition) Check(i v1.Image) error {
	ls, err := i.Layers()
	if err != nil {
		return err
	}
	var rc io.ReadCloser
	// If there's only one layer, we don't need to extract it.
	if len(ls) == 1 {
		rc, err = ls[0].Uncompressed()
		if err != nil {
			return err
		}
	} else {
		rc = mutate.Extract(i)
	}

	tmp, err := os.CreateTemp("", "structure-test")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	defer rc.Close()

	size, err := io.Copy(tmp, rc)
	if err != nil {
		return err
	}

	fsys, err := tarfs.New(tmp, size)
	if err != nil {
		return err
	}

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
			if ts, err := tf.Stat(); err == nil && ts.Size() > maxRegexFileSize {
				errs = append(errs, fmt.Errorf("file %q too large to match regex (max %d bytes)", path, maxRegexFileSize))
				continue
			}

			// We care about the contents, so read and buffer them and regexp.
			got, err := io.ReadAll(io.LimitReader(tf, maxRegexFileSize))
			if err != nil {
				errs = append(errs, fmt.Errorf("reading %q: %w", path, err))
				continue
			}

			// Reject binary files, which are unlikely to match the regex.
			// To determine if a file is binary, we check for null bytes in the first 8KB.
			if bytes.Contains(got[:min(len(got), 8192)], []byte{0}) {
				errs = append(errs, fmt.Errorf("file %q contains binary data", path))
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

func (d DirsCondition) Check(i v1.Image) error {
	ls, err := i.Layers()
	if err != nil {
		return err
	}
	var rc io.ReadCloser
	// If there's only one layer, we don't need to extract it.
	if len(ls) == 1 {
		rc, err = ls[0].Uncompressed()
		if err != nil {
			return err
		}
	} else {
		rc = mutate.Extract(i)
	}

	tmp, err := os.CreateTemp("", "structure-test")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	defer rc.Close()

	size, err := io.Copy(tmp, rc)
	if err != nil {
		return err
	}

	fsys, err := tarfs.New(tmp, size)
	if err != nil {
		return err
	}

	var errs []error

	for path, dir := range d.Want {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(path, "/")

		if !dir.Recursive {
			fi, err := fsys.Stat(name)
			if err != nil {
				errs = append(errs, err)
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
					return err
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
					errs = append(errs, err)
				}

				got := fi.Mode() & permissionMask
				want := *dir.Mode & permissionMask

				if got != want {
					errs = append(errs, fmt.Errorf("file %q mode does not match %o (got %o)", path, want, got))
				}

				return nil
			})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

type PermissionsCondition struct {
	Want map[string]Permission
}

type Permission struct {
	Block    *os.FileMode
	Override []string
}

func (p PermissionsCondition) Check(i v1.Image) error {
	ls, err := i.Layers()
	if err != nil {
		return err
	}
	var rc io.ReadCloser
	// If there's only one layer, we don't need to extract it.
	if len(ls) == 1 {
		rc, err = ls[0].Uncompressed()
		if err != nil {
			return err
		}
	} else {
		rc = mutate.Extract(i)
	}

	tmp, err := os.CreateTemp("", "structure-test")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	defer rc.Close()

	size, err := io.Copy(tmp, rc)
	if err != nil {
		return err
	}

	fsys, err := tarfs.New(tmp, size)
	if err != nil {
		return err
	}

	var errs []error

	for path, perm := range p.Want {
		// https://pkg.go.dev/io/fs#ValidPath
		name := strings.TrimPrefix(path, "/")

		err := fs.WalkDir(fsys, name, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			// ignore symlinks which will register as 777
			if d.Type()&fs.ModeSymlink == fs.ModeSymlink {
				return nil
			}

			fi, err := d.Info()
			if err != nil {
				errs = append(errs, err)
			}

			got := fi.Mode() & permissionMask
			block := *perm.Block & permissionMask

			if got == block && !hasOverride(path, perm.Override) {
				errs = append(errs, fmt.Errorf("file %q mode matches blocked permission %o (got %o)", path, block, got))
			}

			return nil
		})
		if err != nil {
			errs = append(errs, err)
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
