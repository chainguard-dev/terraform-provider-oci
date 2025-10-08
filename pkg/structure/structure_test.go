package structure

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func TestFilesCondition_RegexValidation(t *testing.T) {
	for _, tt := range []struct {
		desc     string
		contents []byte
		wantErr  bool
	}{{
		desc:     "text file with null byte should error",
		contents: []byte("hello\x00world"),
		wantErr:  true,
	}, {
		desc:     "text file without null byte should not error",
		contents: []byte("hello world"),
		wantErr:  false,
	}, {
		desc:     "binary data in first 8KB should error",
		contents: append([]byte("header"), bytes.Repeat([]byte{0}, 100)...),
		wantErr:  true,
	}, {
		desc:     "null byte beyond 8KB should not error",
		contents: append(bytes.Repeat([]byte("a"), 8193), 0),
		wantErr:  false,
	}, {
		desc:     "file at 1MB limit should not error",
		contents: bytes.Repeat([]byte("a"), 1*1024*1024),
		wantErr:  false,
	}, {
		desc:     "file over 1MB should error",
		contents: bytes.Repeat([]byte("a"), 1*1024*1024+1),
		wantErr:  true,
	}} {
		t.Run(tt.desc, func(t *testing.T) {
			img := createImageWithFile(t, "/test.txt", tt.contents, 0644)

			fc := FilesCondition{
				Want: map[string]File{
					"/test.txt": {
						Regex: ".*",
					},
				},
			}

			if err := fc.Check(img); tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// createImageWithFile creates a test image with a single file
func createImageWithFile(t *testing.T, path string, contents []byte, mode os.FileMode) v1.Image {
	t.Helper()

	// Create a tar archive with the file
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: path[1:], // Remove leading slash for tar
		Mode: int64(mode),
		Size: int64(len(contents)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	// Create layer from tar archive
	layer, err := tarball.LayerFromReader(&buf)
	if err != nil {
		t.Fatal(err)
	}

	// Create image with the layer
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatal(err)
	}

	return img
}
