package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var digestRE = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

func TestAccRefDataSource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	ref := repo.Tag("latest")
	t.Logf("Using ref: %s", ref)
	img, err := random.Image(1024, 3)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	img = mutate.MediaType(img, ggcrtypes.OCIManifestSchema1)
	img = mutate.Annotations(img, map[string]string{ //nolint:forcetypeassert
		"foo": "bar",
	}).(v1.Image)
	if err := remote.Write(ref, img); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	d, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}

	// An image specified by tag has a .tag attribute, and all the other image manifest attributes.

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_ref" "test" { ref = %q }`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "digest", d.String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "tag", "latest"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.schema_version", "2"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.media_type", string(ggcrtypes.OCIManifestSchema1)),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.config.digest", digestRE),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.layers.#", "3"),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.layers.0.digest", digestRE),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.layers.1.digest", digestRE),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.layers.2.digest", digestRE),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.annotations.foo", "bar"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.annotations.bar"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.manifests"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.subject"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "images"),
				),
			},
		},
	})

	// A ref specified by digest has no .tag attribute.
	refByDigest := ref.Context().Digest(d.String())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_ref" "test" { ref = %q }`, refByDigest),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "digest", d.String()),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "tag"),
				),
			},
		},
	})

	// Push an index to the local registry.
	var idx v1.ImageIndex = empty.Index
	for _, plat := range []v1.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "windows", Architecture: "arm64", Variant: "v3", OSVersion: "1-rc365"},
	} {
		plat := plat
		img, err := random.Image(1024, 3)
		if err != nil {
			t.Fatalf("failed to create image: %v", err)
		}
		img = mutate.MediaType(img, ggcrtypes.OCIManifestSchema1)
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add:        img,
			Descriptor: v1.Descriptor{Platform: &plat},
		})
	}
	idx = mutate.IndexMediaType(idx, ggcrtypes.OCIImageIndex)
	idx = mutate.Annotations(idx, map[string]string{ //nolint:forcetypeassert
		"foo": "bar",
	}).(v1.ImageIndex)

	ref = repo.Tag("index")
	t.Logf("Using ref: %s", ref)
	if err := remote.WriteIndex(ref, idx); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	rmf, err := idx.RawManifest()
	if err != nil {
		t.Fatalf("failed to get index raw manifest: %v", err)
	}
	t.Log(string(rmf))

	d, err = idx.Digest()
	if err != nil {
		t.Fatalf("failed to get index digest: %v", err)
	}

	// An index specified by tag has a .tag attribute, and all the other index manifest attributes.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_ref" "test" { ref = %q }`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "digest", d.String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "tag", "index"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.schema_version", "2"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.media_type", string(ggcrtypes.OCIImageIndex)),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.#", "2"),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.manifests.0.digest", digestRE),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.0.platform.os", "linux"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.0.platform.architecture", "amd64"),
					resource.TestMatchResourceAttr("data.oci_ref.test", "manifest.manifests.1.digest", digestRE),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.1.platform.os", "windows"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.1.platform.architecture", "arm64"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.1.platform.variant", "v3"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.manifests.1.platform.os_version", "1-rc365"),
					resource.TestCheckResourceAttr("data.oci_ref.test", "images.%", "2"),
					resource.TestMatchResourceAttr("data.oci_ref.test", "images.linux/amd64.digest", digestRE),
					resource.TestMatchResourceAttr("data.oci_ref.test", "images.windows/arm64/v3:1-rc365.digest", digestRE),
					resource.TestCheckResourceAttr("data.oci_ref.test", "manifest.annotations.foo", "bar"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.annotations.bar"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.config"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.layers"),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "manifest.subject"),
				),
			},
		},
	})
}
