package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExampleResource(t *testing.T) {
	reg, cleanup := setupRegistry(t)
	defer cleanup()

	ref1, err := name.ParseReference(reg + "/test:1")
	if err != nil {
		t.Fatalf("failed to parse reference: %v", err)
	}
	t.Logf("Using ref1: %s", ref1)

	// Push an image to the local registry.
	img1, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := remote.Write(ref1, img1); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	ref2, err := name.ParseReference(reg + "/test:2")
	if err != nil {
		t.Fatalf("failed to parse reference: %v", err)
	}
	t.Logf("Using ref2: %s", ref2)

	// Push an image to the local registry.
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
					resource.TestMatchResourceAttr("oci_append.test", "id", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
				),
			},
			// ImportState testing
			{
				ResourceName:      "oci_append.test",
				ImportState:       true,
				ImportStateVerify: true,
				// This is not normally necessary, but is here because this
				// example code does not have an actual upstream service.
				// Once the Read method is able to refresh information from
				// the upstream service, this can be removed.
				ImportStateVerifyIgnore: []string{"base_image", "layers"},
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
