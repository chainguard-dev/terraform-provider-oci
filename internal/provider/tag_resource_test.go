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
	repo, cleanup := ocitesting.SetupRepository(t, "repo")
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

	// Push another image to the local registry.
	ref2 := repo.Tag("2")
	t.Logf("Using ref2: %s", ref2)
	img2, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := remote.Write(ref2, img2); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}
	d2, err := img2.Digest()
	if err != nil {
		t.Fatalf("failed to get digest: %v", err)
	}
	dig2 := ref2.Context().Digest(d2.String())

	// Create the resource tagging dig1 three tags.
	want1 := fmt.Sprintf("%s:foo@%s", repo, d1)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "oci_tag" "test" {
				  digest_ref = %q
				  tags       = ["foo", "bar", "baz"]
				}`, dig1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want1),
					resource.TestCheckResourceAttr("oci_tag.test", "id", want1),
				),
			},
		},
	})

	// The digest should be tagged with all three tags.
	for _, want := range []string{"foo", "bar", "baz"} {
		desc, err := remote.Get(repo.Tag(want))
		if err != nil {
			t.Errorf("failed to get image with tag %q: %v", want, err)
		}
		if desc.Digest != d1 {
			t.Errorf("image with tag %q has wrong digest: got %s, want %s", want, desc.Digest, d1)
		}
	}

	// Point the tags to another digest.
	want2 := fmt.Sprintf("%s:foo@%s", repo, d2)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "oci_tag" "test" {
				  digest_ref = %q
				  tags       = ["foo", "bar", "baz"]
				}`, dig2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want2),
					resource.TestCheckResourceAttr("oci_tag.test", "id", want2),
				),
			},
		},
	})

	// The second digest should be tagged with all three tags.
	for _, want := range []string{"foo", "bar", "baz"} {
		desc, err := remote.Get(repo.Tag(want))
		if err != nil {
			t.Errorf("failed to get image with tag %q: %v", want, err)
		}
		if desc.Digest != d2 {
			t.Errorf("image with tag %q has wrong digest: got %s, want %s", want, desc.Digest, d2)
		}
	}

	// Add a fourth tag.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "oci_tag" "test" {
					  digest_ref = %q
					  tags       = ["foo", "bar", "baz", "qux"]
					}`, dig2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want2),
					resource.TestCheckResourceAttr("oci_tag.test", "id", want2),
				),
			},
		},
	})

	// The second digest should be tagged with all three tags.
	for _, want := range []string{"foo", "bar", "baz", "qux"} {
		desc, err := remote.Get(repo.Tag(want))
		if err != nil {
			t.Errorf("failed to get image with tag %q: %v", want, err)
		}
		if desc.Digest != d2 {
			t.Errorf("image with tag %q has wrong digest: got %s, want %s", want, desc.Digest, d2)
		}
	}

	// Tag the digest with the same tag multiple times, which should be allowed but warn.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`resource "oci_tag" "test" {
					  digest_ref = %q
					  tags       = ["foo", "foo", "foo", "bar", "bar", "bar"]
					}`, dig2),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("oci_tag.test", "tagged_ref", want2),
				resource.TestCheckResourceAttr("oci_tag.test", "id", want2),
			),
		}},
	})
}
