package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAppendResource(t *testing.T) {
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

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: fmt.Sprintf(`resource "oci_append" "test" {
				  base_image = %q
				  layers = [{
					files = {
					  "/usr/local/test.txt" = { contents = "hello world" }
					}
				  }]
				}`, ref1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_append.test", "base_image", ref1.String()),
					resource.TestMatchResourceAttr("oci_append.test", "image_ref", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
					resource.TestMatchResourceAttr("oci_append.test", "id", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
				),
			},
			// Update and Read testing
			{
				Config: fmt.Sprintf(`resource "oci_append" "test" {
					base_image = %q
					layers = [{
					  files = {
						"/usr/local/test.txt" = { contents = "hello world" }
						"/usr/bin/test.sh"    = { contents = "echo hello world" }
					  }
					}]
				  }`, ref2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_append.test", "base_image", ref2.String()),
					resource.TestMatchResourceAttr("oci_append.test", "id", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
