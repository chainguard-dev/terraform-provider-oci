package provider

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccStructureTestDataSource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	// File tests
	_ = tw.WriteHeader(&tar.Header{
		Name: "foo",
		Mode: 0o644,
		Size: 3,
	})
	_, _ = tw.Write([]byte("bar"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/bar",
		Mode: 0o666,
		Size: 19,
	})
	_, _ = tw.Write([]byte("world-writable file"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/barexec",
		Mode: 0o777,
		Size: 25,
	})
	_, _ = tw.Write([]byte("world-writable executable"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/baz",
		Mode: 0o755,
		Size: 6,
	})
	_, _ = tw.Write([]byte("blah!!"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/stickybit",
		Mode: int64(0o755 | 0o1000),
		Size: 20,
	})
	_, _ = tw.Write([]byte("file with sticky bit"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/setgid",
		Mode: int64(0o755 | 0o2000),
		Size: 16,
	})
	_, _ = tw.Write([]byte("file with setgid"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/setuid",
		Mode: int64(0o755 | 0o4000),
		Size: 16,
	})
	_, _ = tw.Write([]byte("file with setuid"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/setuidgid",
		Mode: int64(0o755 | 0o4000 | 0o2000),
		Size: 27,
	})
	_, _ = tw.Write([]byte("file with setuid and setgid"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/setuidgidstickybit",
		Mode: int64(0o755 | 0o4000 | 0o2000 | 0o1000),
		Size: 40,
	})
	_, _ = tw.Write([]byte("file with setuid, setgid, and sticky bit"))
	// Directory tests
	_ = tw.WriteHeader(&tar.Header{
		Name:     "new_path",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name:     "new_path_permissive",
		Typeflag: tar.TypeDir,
		Mode:     0o777,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name:     "new_path_permissive/foo",
		Typeflag: tar.TypeDir,
		Mode:     0o777,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name: "new_path_permissive/foo/bar",
		Mode: 0o777,
		Size: 6,
	})
	_, _ = tw.Write([]byte("blah!!"))
	_ = tw.WriteHeader(&tar.Header{
		Name:     "new_path_permissive/bar",
		Typeflag: tar.TypeDir,
		Mode:     0o777,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name: "new_path_permissive/bar/baz",
		Mode: 0o777,
		Size: 6,
	})
	_, _ = tw.Write([]byte("blah!!"))
	_ = tw.WriteHeader(&tar.Header{
		Name:     "files_only/foo",
		Typeflag: tar.TypeDir,
		Mode:     0o777,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name:     "files_only/foo/bar",
		Typeflag: tar.TypeDir,
		Mode:     0o777,
	})
	_ = tw.WriteHeader(&tar.Header{
		Name: "files_only/foo/baz",
		Mode: 0o644,
		Size: 6,
	})
	_, _ = tw.Write([]byte("blah!!"))

	// Test that /lib -> /usr/lib works.
	_ = tw.WriteHeader(&tar.Header{
		Name:     "symlink",
		Typeflag: tar.TypeSymlink,
		Mode:     0o755,
		Linkname: "path",
	})

	tw.Close()

	l, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewBuffer(buf.Bytes())), nil
	})
	if err != nil {
		t.Fatalf("failed to create layer: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, l)
	if err != nil {
		t.Fatalf("failed to append layer: %v", err)
	}
	img, err = mutate.Config(img, v1.Config{
		Env: []string{"FOO=bar", "BAR=baz"},
	})
	if err != nil {
		t.Fatalf("failed to mutate image: %v", err)
	}
	idx := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})
	d, err := idx.Digest()
	if err != nil {
		t.Fatalf("failed to get index digest: %v", err)
	}
	ref := repo.Digest(d.String())
	if err := remote.WriteIndex(ref, idx); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`data "oci_structure_test" "test" {
  digest = %q

  conditions {
    env {
      key = "FOO"
      value = "bar"
    }
    env {
      key = "BAR"
      value = "baz"
    }
    files {
      path  = "/foo"
      regex = "bar"
      mode  = "0644"
    }
    files {
      path = "/foo" # Just test existence.
    }
    files {
      path  = "/foo"
      regex = "b[ar]+" # Test regexp.
    }
    files {
      path  = "/path/to/baz"
      regex = "blah!!"
      mode  = "0755"
    }
    files {
      path  = "/path/to/bar"
      regex = "world-writable file"
      mode  = "0666"
    }
    files {
      path  = "/path/to/barexec"
      regex = "world-writable executable"
      mode  = "0777"
    }
    files {
      path  = "/path/to/stickybit"
      regex = "file with sticky bit"
      mode  = "1755"
    }
    files {
      path  = "/path/to/setgid"
      regex = "file with setgid"
      mode  = "2755"
    }
    files {
      path  = "/path/to/setuid"
      regex = "file with setuid"
      mode  = "4755"
    }
    files {
      path  = "/path/to/setuidgid"
      regex = "file with setuid and setgid"
      mode  = "6755"
    }
    files {
      path  = "/path/to/setuidgidstickybit"
      regex = "file with setuid, setgid, and sticky bit"
      mode  = "7755"
    }
    files {
      path  = "/path/to/mayormaynotexist"
      regex = "file that may exist for one image and not another"
      mode  = "0644"
      optional = true
    }
    dirs {
      path = "/new_path"
      mode = "0755"
    }
    dirs {
      path = "/new_path_permissive"
      mode = "0777"
    }
    dirs {
      path = "/new_path_permissive"
      mode = "0777"
      recursive = true
    }
    dirs {
      path = "/files_only/foo"
      mode = "0644"
      recursive = true
      files_only = true
    }
    files {
      path  = "/symlink/to/baz"
      regex = "blah!!"
    }
    permissions {
      block = "0777"
      override = ["/new_path_permissive", "/path/to/barexec"]
    }
    permissions {
      block = "0666"
    }
  }
}`, ref),
			Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("data.oci_structure_test.test", "digest", ref.String()),
				resource.TestCheckResourceAttr("data.oci_structure_test.test", "id", ref.String()),
			),
		}},
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`data "oci_structure_test" "test" {
  digest = %q

  conditions {
    env {
      key = "NOT_SET"
      value = "uh oh"
    }
    files {
      path = "/path/not/set"
    }
  }
}`, ref),
			ExpectError: regexp.MustCompile(`env "NOT_SET" does not match "uh oh" \(got ""\)\n.*file "/path/not/set" not found`),
		}},
	})
}

func TestInvalidPathEnv(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{
		Name: "foo",
		Mode: 0o644,
		Size: 3,
	})
	_, _ = tw.Write([]byte("bar"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/baz",
		Mode: 0o755,
		Size: 6,
	})
	_, _ = tw.Write([]byte("blah!!"))
	tw.Close()

	l, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewBuffer(buf.Bytes())), nil
	})
	if err != nil {
		t.Fatalf("failed to create layer: %v", err)
	}

	img, err := mutate.AppendLayers(empty.Image, l)
	if err != nil {
		t.Fatalf("failed to append layer: %v", err)
	}
	img, err = mutate.Config(img, v1.Config{
		Env: []string{
			"PATH=$PATH",
			"LUA_PATH=baz;/whatever;$LUA_PATH",
		},
	})
	if err != nil {
		t.Fatalf("failed to mutate image: %v", err)
	}
	idx := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: img})
	d, err := idx.Digest()
	if err != nil {
		t.Fatalf("failed to get index digest: %v", err)
	}
	ref := repo.Digest(d.String())
	if err := remote.WriteIndex(ref, idx); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`data "oci_structure_test" "test" {
	  digest = %q

	  conditions {
			env {
				key = "PATH"
				value = "$PATH"
			}

			env {
				key = "LUA_PATH"
				value = "baz;/whatever;$LUA_PATH"
			}
		}
	}`, ref),
			ExpectError: regexp.MustCompile(`env "PATH" value "\$PATH" references relative path or literal \$ string "\$PATH"\nenv "LUA_PATH" value "baz;/whatever;\$LUA_PATH" references relative path or\nliteral \$ string "baz"\nenv "LUA_PATH" value "baz;/whatever;\$LUA_PATH" references relative path or\nliteral \$ string "\$LUA_PATH"`),
		}},
	})
}

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		modeStr string
		want    os.FileMode
	}{
		{"1", 0o001},
		{"75", 0o075},
		{"777", 0o777},
		{"644", 0o644},
		{"666", 0o666},
		{"0644", 0o644},
		{"0755", 0o755},
		{"0777", 0o777},
		{"1755", 0o755 | os.ModeSticky},
		{"2755", 0o755 | os.ModeSetgid},
		{"4755", 0o755 | os.ModeSetuid},
		{"6755", 0o755 | os.ModeSetuid | os.ModeSetgid},
		{"7755", 0o755 | os.ModeSetuid | os.ModeSetgid | os.ModeSticky},
		{"0000", 0o000},
	}

	for _, tt := range tests {
		t.Run(tt.modeStr, func(t *testing.T) {
			got, err := parseFileMode(tt.modeStr)
			if err != nil {
				t.Fatalf("parseFileMode(%q) returned error: %v", tt.modeStr, err)
			}
			if got == nil || *got != tt.want {
				t.Errorf("parseFileMode(%q) = %v, want %v", tt.modeStr, got, tt.want)
			}
		})
	}

	// Test unset -> nil
	t.Run("unset", func(t *testing.T) {
		got, err := parseFileMode("")
		if err != nil {
			t.Fatalf("parseFileMode(\"\") returned error: %v", err)
		}
		if got != nil {
			t.Errorf("parseFileMode(\"\") = %v, want nil", got)
		}
	})

	t.Run("invalid mode", func(t *testing.T) {
		_, err := parseFileMode("invalid")
		if err == nil {
			t.Error("parseFileMode(\"invalid\") did not return an error")
		}
	})

	t.Run("invalid numerical mode", func(t *testing.T) {
		_, err := parseFileMode("0999")
		if err == nil {
			t.Error("parseFileMode(\"0999\") did not return an error")
		}
	})

	t.Run("invalid octal mode", func(t *testing.T) {
		_, err := parseFileMode("0o777")
		if err == nil {
			t.Error("parseFileMode(\"0o777\") did not return an error")
		}
	})
}
