package structure

import (
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
	Regex string
	Mode  *os.FileMode
}

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
			if errors.Is(err, fs.ErrNotExist) {
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
			if stat.Mode() != *f.Mode {
				errs = append(errs, fmt.Errorf("file %q mode does not match %o (got %o)", path, *f.Mode, stat.Mode()))
			}
		}
	}

	return errors.Join(errs...)
}
