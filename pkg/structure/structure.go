package structure

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
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
	// TODO: support filemode
	ran bool
}

func (f FilesCondition) Check(i v1.Image) error {
	rc := mutate.Extract(i)
	defer rc.Close()
	tr := tar.NewReader(rc)
	errs := []error{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if !strings.HasPrefix(hdr.Name, "/") {
			hdr.Name = "/" + hdr.Name
		}

		if _, found := f.Want[hdr.Name]; !found {
			// We don't care about this file at all, on to the next.
			continue
		}
		if f.Want[hdr.Name].Regex != "" {
			// We care about the contents, so read and buffer them and regexp.
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				return err
			}
			if !regexp.MustCompile(f.Want[hdr.Name].Regex).Match(buf.Bytes()) {
				errs = append(errs, fmt.Errorf("file %q does not match regexp %q, got:\n%s", hdr.Name, f.Want[hdr.Name].Regex, buf.String()))
			}
		}
		// At least mark that we found this file we cared about.
		f.Want[hdr.Name] = File{
			Regex: f.Want[hdr.Name].Regex,
			ran:   true,
		}
	}
	for path, f := range f.Want {
		if !f.ran {
			errs = append(errs, fmt.Errorf("file %q not found", path))
		}
	}
	return errors.Join(errs...)
}
