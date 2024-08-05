package provider

import (
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
					resource.TestCheckFunc(func(s *terraform.State) error {
						rs := s.RootModule().Resources["oci_append.test"]
						img, err := crane.Pull(rs.Primary.Attributes["image_ref"])
						if err != nil {
							return fmt.Errorf("failed to pull image: %v", err)
						}
						if err := validate.Image(img); err != nil {
							return fmt.Errorf("failed to validate image: %v", err)
						}
						return nil
					}),
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

	// Push an index to the local registry.
	ref3 := repo.Tag("3")
	idx1, err := random.Index(3, 1, 3)
	if err != nil {
		t.Fatalf("failed to create index: %v", err)
	}
	if err := remote.WriteIndex(ref3, idx1); err != nil {
		t.Fatalf("failed to write index: %v", err)
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
      }`, ref3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_append.test", "base_image", ref3.String()),
					resource.TestMatchResourceAttr("oci_append.test", "id", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
					resource.TestCheckFunc(func(s *terraform.State) error {
						rs := s.RootModule().Resources["oci_append.test"]
						ref, err := name.ParseReference(rs.Primary.Attributes["image_ref"])
						if err != nil {
							return fmt.Errorf("failed to parse reference: %v", err)
						}
						idx, err := remote.Index(ref)
						if err != nil {
							return fmt.Errorf("failed to pull image: %v", err)
						}
						if err := validate.Index(idx); err != nil {
							return fmt.Errorf("failed to validate image: %v", err)
						}
						return nil
					}),
				),
			},
		},
	})

	ref4 := repo.Tag("4")
	var idx2 v1.ImageIndex = empty.Index

	idx2 = mutate.AppendManifests(idx2, mutate.IndexAddendum{Add: img1})
	idx2 = mutate.AppendManifests(idx2, mutate.IndexAddendum{Add: img1})

	if err := remote.WriteIndex(ref4, idx2); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: fmt.Sprintf(`
resource "oci_append" "test" {
  base_image = %q
  layers = [{
    files = {
      "/usr/local/test.txt" = { contents = "hello world" }
    }
  }]
}
          `, ref4),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_append.test", "base_image", ref4.String()),
					resource.TestMatchResourceAttr("oci_append.test", "id", regexp.MustCompile(`/test@sha256:[0-9a-f]{64}$`)),
					resource.TestCheckFunc(func(s *terraform.State) error {
						rs := s.RootModule().Resources["oci_append.test"]
						ref, err := name.ParseReference(rs.Primary.Attributes["image_ref"])
						if err != nil {
							return fmt.Errorf("failed to parse reference: %v", err)
						}
						idx, err := remote.Index(ref)
						if err != nil {
							return fmt.Errorf("failed to pull index: %v", err)
						}
						if err := validate.Index(idx); err != nil {
							return fmt.Errorf("failed to validate index: %v", err)
						}

						return nil
					}),
				),
			},
		},
	})
}
