package provider

import (
	"fmt"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRefDataSource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	ref := repo.Tag("latest")
	t.Logf("Using ref: %s", ref)
	img, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := remote.Write(ref, img); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	d, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}

	refByDigest := ref.Context().Digest(d.String())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A ref specified by tag has a .tag attribute.
			{
				Config: fmt.Sprintf(`data "oci_ref" "test" {
				  ref = %q
				}`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "digest", d.String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "tag", "latest"),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A ref specified by digest has no .tag attribute.
			{
				Config: fmt.Sprintf(`data "oci_ref" "test" {
				  ref = %q
				}`, refByDigest),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.oci_ref.test", "digest", d.String()),
					resource.TestCheckNoResourceAttr("data.oci_ref.test", "tag"),
				),
			},
		},
	})
}
