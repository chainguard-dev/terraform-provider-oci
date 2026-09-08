package structure

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"slices"
	"testing"
)

func testTar(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write := func(hdr *tar.Header, body string) {
		hdr.Size = int64(len(body))
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, body); err != nil {
			t.Fatal(err)
		}
	}
	write(&tar.Header{Name: "./etc/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	write(&tar.Header{Name: "etc/os-release", Typeflag: tar.TypeReg, Mode: 0o644}, "PRETTY_NAME=\"Wolfi\"\n")
	write(&tar.Header{Name: "etc/hosts", Typeflag: tar.TypeReg, Mode: 0o666}, "127.0.0.1 localhost\n")
	write(&tar.Header{Name: "etc/alias", Typeflag: tar.TypeSymlink, Mode: 0o777, Linkname: "hosts"}, "")
	write(&tar.Header{Name: "etc/abs", Typeflag: tar.TypeSymlink, Mode: 0o777, Linkname: "/etc/os-release"}, "")
	write(&tar.Header{Name: "usr/lib/libfoo.so", Typeflag: tar.TypeReg, Mode: 0o755}, "ELF")
	write(&tar.Header{Name: "lib", Typeflag: tar.TypeSymlink, Mode: 0o777, Linkname: "usr/lib"}, "")
	write(&tar.Header{Name: "usr/lib/hard", Typeflag: tar.TypeLink, Mode: 0o755, Linkname: "etc/hosts"}, "")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTarIndex(t *testing.T) {
	raw := testTar(t)
	idx, err := newTarIndex(bytes.NewReader(raw), map[string]bool{"etc/os-release": true, "etc/hosts": true, "usr/lib/libfoo.so": true})
	if err != nil {
		t.Fatal(err)
	}

	readFile := func(name string) string {
		t.Helper()
		f, err := idx.Open(name)
		if err != nil {
			t.Fatalf("open %q: %v", name, err)
		}
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		return string(b)
	}

	// Captured content is served from memory.
	if got := readFile("etc/os-release"); got != "PRETTY_NAME=\"Wolfi\"\n" {
		t.Errorf("os-release = %q", got)
	}

	// Symlinks: relative, absolute, symlinked parent directory, hard link.
	if got := readFile("etc/alias"); got != "127.0.0.1 localhost\n" {
		t.Errorf("relative symlink = %q", got)
	}
	if got := readFile("etc/abs"); got != "PRETTY_NAME=\"Wolfi\"\n" {
		t.Errorf("absolute symlink = %q", got)
	}
	if got := readFile("lib/libfoo.so"); got != "ELF" {
		t.Errorf("symlinked dir = %q", got)
	}
	if got := readFile("usr/lib/hard"); got != "127.0.0.1 localhost\n" {
		t.Errorf("hard link = %q", got)
	}

	// Open reports the mode of the resolved target, Stat does not follow.
	f, _ := idx.Open("etc/alias")
	if fi, _ := f.Stat(); fi.Mode()&permissionMask != 0o666 {
		t.Errorf("open(alias) mode = %o", fi.Mode()&permissionMask)
	}
	if fi, err := idx.Stat("etc/alias"); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("stat(alias) = %v, %v, want symlink", fi, err)
	}

	// Missing files.
	if _, err := idx.Open("nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("open(nope) = %v, want ErrNotExist", err)
	}
	if _, err := idx.Stat("nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat(nope) = %v, want ErrNotExist", err)
	}

	// Directory listings are sorted and include implicit directories.
	names := func(name string) []string {
		t.Helper()
		des, err := idx.ReadDir(name)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, d := range des {
			out = append(out, d.Name())
		}
		return out
	}
	if got := names("etc"); !slices.Equal(got, []string{"abs", "alias", "hosts", "os-release"}) {
		t.Errorf("readdir(etc) = %v", got)
	}
	if got := names("usr/lib"); !slices.Equal(got, []string{"hard", "libfoo.so"}) {
		t.Errorf("readdir(usr/lib) = %v", got)
	}
	if got := names("."); !slices.Equal(got, []string{"etc", "lib"}) {
		t.Errorf("readdir(.) = %v", got)
	}

	// Walking from the root reaches the explicit directory and its children.
	var walked []string
	if err := fs.WalkDir(idx, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{".", "etc", "etc/abs", "etc/alias", "etc/hosts", "etc/os-release", "lib"}
	if !slices.Equal(walked, want) {
		t.Errorf("walk = %v, want %v", walked, want)
	}
}

func TestTarIndexUncaptured(t *testing.T) {
	raw := testTar(t)
	idx, err := newTarIndex(bytes.NewReader(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Opening and statting work without contents, reading does not.
	f, err := idx.Open("etc/hosts")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := f.Stat(); err != nil || fi.Mode()&permissionMask != 0o666 {
		t.Errorf("stat = %v, %v", fi, err)
	}
	if _, err := io.ReadAll(f); err == nil {
		t.Error("read of uncaptured file succeeded, want error")
	}
}
