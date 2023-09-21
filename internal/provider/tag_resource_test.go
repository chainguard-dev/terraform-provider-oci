package provider

import (
	"fmt"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTagResource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	ref1 := repo.Tag("1")
	t.Logf("Using ref1: %s", ref1)
	img1, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := remote.Write(ref1, img1); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}
	d1, err := img1.Digest()
	if err != nil {
		t.Fatalf("failed to get digest: %v", err)
	}
	dig1 := ref1.Context().Digest(d1.String())

	// Push an image to the local registry.
	ref2 := repo.Tag("2")
	t.Logf("Using ref2: %s", ref2)
	img2, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := remote.Write(ref2, img2); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}
	d2, err := img1.Digest()
	if err != nil {
		t.Fatalf("failed to get digest: %v", err)
	}
	dig2 := ref2.Context().Digest(d2.String())

	want1 := fmt.Sprintf("%s:test@%s", repo, d2)
	want2 := fmt.Sprintf("%s:test2@%s", repo, d2)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: fmt.Sprintf(`resource "oci_tag" "test" {
				  digest_ref = %q
				  tag        = "test"
				}`, dig1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want1),
					resource.TestCheckResourceAttr("oci_tag.test", "id", want1),
				),
			},
			// Update and Read testing
			{
				Config: fmt.Sprintf(`resource "oci_tag" "test" {
					digest_ref = %q
					tag        = "test2"
				  }`, dig2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want2),
					resource.TestCheckResourceAttr("oci_tag.test", "id", want2),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
