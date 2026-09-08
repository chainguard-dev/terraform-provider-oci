package structure

import (
	"archive/tar"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// tarIndex is an in-memory view of a tar stream. It records the header of every
// entry so that metadata questions (existence, mode, type, directory listings)
// can be answered without keeping the tar around, and it captures the contents
// of a chosen set of paths during the single pass over the stream. Contents of
// any other regular file can still be read on demand, at the cost of streaming
// the tar a second time via reopen.
type tarIndex struct {
	entries map[string]*tarEntry
	dirs    map[string][]fs.DirEntry

	// reopen returns a fresh reader over the same tar stream. It is only used
	// when a caller opens a regular file whose contents were not captured.
	reopen func() (io.ReadCloser, error)
}

type tarEntry struct {
	hdr  tar.Header
	name string // normalized path within the tar
	dir  string // path.Dir(name)
	fi   fs.FileInfo

	captured bool
	data     []byte
}

func (e *tarEntry) Name() string               { return e.fi.Name() }
func (e *tarEntry) IsDir() bool                { return e.fi.IsDir() }
func (e *tarEntry) Type() fs.FileMode          { return e.fi.Mode().Type() }
func (e *tarEntry) Info() (fs.FileInfo, error) { return e.fi, nil }

// newTarIndex consumes r to completion, indexing every header. Contents are
// retained for entries whose normalized path is in capture.
func newTarIndex(r io.Reader, capture map[string]bool, reopen func() (io.ReadCloser, error)) (*tarIndex, error) {
	idx := &tarIndex{
		entries: map[string]*tarEntry{},
		dirs:    map[string][]fs.DirEntry{},
		reopen:  reopen,
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		name := normalize(hdr.Name)
		e := &tarEntry{
			hdr:  *hdr,
			name: name,
			dir:  path.Dir(name),
			fi:   hdr.FileInfo(),
		}

		if capture[name] && hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("reading %q: %w", name, err)
			}
			e.captured = true
			e.data = data
		}

		idx.entries[name] = e
		idx.dirs[e.dir] = append(idx.dirs[e.dir], e)
	}

	for _, children := range idx.dirs {
		slices.SortFunc(children, func(a, b fs.DirEntry) int {
			return cmp.Compare(a.Name(), b.Name())
		})
	}

	return idx, nil
}

// normalize strips a leading "/" or "./" and a trailing "/" so that paths from
// tar headers and from callers compare equal.
func normalize(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSuffix(s, "/"), "/"), "./")
}

type rootInfo struct{}

func (rootInfo) Name() string       { return "." }
func (rootInfo) Size() int64        { return 0 }
func (rootInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (rootInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (rootInfo) IsDir() bool        { return true }
func (rootInfo) Sys() any           { return nil }

// Stat implements fs.StatFS. It does not follow symlinks.
func (idx *tarIndex) Stat(name string) (fs.FileInfo, error) {
	if e, ok := idx.entries[name]; ok {
		return e.fi, nil
	}
	// fs.WalkDir expects "." to return a root entry to bootstrap the walk.
	if name == "." {
		return rootInfo{}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// ReadDir implements fs.ReadDirFS. Directories that only exist implicitly, as
// the parent of some entry, still list their children.
func (idx *tarIndex) ReadDir(name string) ([]fs.DirEntry, error) {
	children, ok := idx.dirs[name]
	if !ok {
		return []fs.DirEntry{}, nil
	}
	return children, nil
}

// Arbitrary limit borrowed from filepath.EvalSymlinks.
const maxHops = 255

// Open implements fs.FS, following symlinks and hard links.
func (idx *tarIndex) Open(name string) (fs.File, error) {
	if name == "." {
		return &tarFile{fi: rootInfo{}, r: bytes.NewReader(nil)}, nil
	}
	return idx.open(name, 0)
}

func (idx *tarIndex) open(name string, hops int) (fs.File, error) {
	if hops > maxHops {
		return nil, fmt.Errorf("opening %s: chased too many (%d) symlinks", name, maxHops)
	}

	e, ok := idx.entries[name]
	if !ok {
		// Deal with symlinked parent directories.
		for dir := range parentDirs(name) {
			p, ok := idx.entries[dir]
			if !ok || p.hdr.Typeflag != tar.TypeSymlink {
				continue
			}

			rest := strings.TrimPrefix(name, dir)
			link := p.hdr.Linkname
			if path.IsAbs(link) {
				return idx.open(normalize(path.Join(link, rest)), hops+1)
			}
			return idx.open(path.Join(p.dir, link, rest), hops+1)
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	switch e.hdr.Typeflag {
	case tar.TypeSymlink, tar.TypeLink:
		link := e.hdr.Linkname
		if path.IsAbs(link) || e.hdr.Typeflag == tar.TypeLink {
			return idx.open(normalize(link), hops+1)
		}
		return idx.open(path.Join(e.dir, link), hops+1)
	}

	if e.hdr.Typeflag == tar.TypeReg && !e.captured {
		if err := idx.load(e); err != nil {
			return nil, fmt.Errorf("opening %s: %w", name, err)
		}
	}

	return &tarFile{fi: e.fi, r: bytes.NewReader(e.data)}, nil
}

// load streams the tar again to fetch the contents of a single entry that was
// not captured during indexing.
func (idx *tarIndex) load(e *tarEntry) error {
	if idx.reopen == nil {
		return errors.New("contents were not captured and the tar cannot be reopened")
	}

	rc, err := idx.reopen()
	if err != nil {
		return fmt.Errorf("reopening tar: %w", err)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("entry %q not found on reopen", e.name)
		}
		if err != nil {
			return err
		}
		if normalize(hdr.Name) != e.name {
			continue
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("reading %q: %w", e.name, err)
		}
		e.captured = true
		e.data = data
		return nil
	}
}

// parentDirs yields every proper ancestor of name, shortest first.
func parentDirs(name string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for i, v := range name {
			if v == '/' {
				if !yield(name[:i]) {
					return
				}
			}
		}
	}
}

type tarFile struct {
	fi fs.FileInfo
	r  *bytes.Reader
}

func (f *tarFile) Stat() (fs.FileInfo, error) { return f.fi, nil }
func (f *tarFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *tarFile) Close() error               { return nil }
