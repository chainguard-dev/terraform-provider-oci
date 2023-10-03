package provider

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
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
	_ = tw.WriteHeader(&tar.Header{
		Name: "foo",
		Mode: 0644,
		Size: 3,
	})
	_, _ = tw.Write([]byte("bar"))
	_ = tw.WriteHeader(&tar.Header{
		Name: "path/to/baz",
		Mode: 0755,
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
