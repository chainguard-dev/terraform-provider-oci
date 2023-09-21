package provider

import (
	"encoding/json"
	"fmt"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTagsResource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "repo")
	defer cleanup()

	// Push an image to the local registry.
	ref1 := repo.Tag("1")
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
	t.Logf("Using ref1: %s -> %s", ref1, dig1)

	// Push another image to the local registry.
	ref2 := repo.Tag("2")
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
	t.Logf("Using ref2: %s -> %s", ref2, dig2)

	// Tag the digests with some tags.
	marshal := func(a any) string {
		b, err := json.MarshalIndent(a, "", "  ")
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		return string(b)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`resource "oci_tags" "test" {
				repo = %q
				tags = %s
			}`, repo, marshal(map[string]v1.Hash{
				"foo":   d1,
				"bar":   d1,
				"baz":   d1,
				"hello": d2,
				"world": d2,
			})),
		}},
	})

	// Check those tags were applied, and the original tags didn't change.
	checkTags := func(want map[string][]string) {
		for dig, tags := range want {
			d, err := name.NewDigest(dig)
			if err != nil {
				t.Fatalf("error parsing digest ref: %v", err)
			}
			for _, tag := range tags {
				got, err := remote.Head(repo.Tag(tag))
				if err != nil {
					t.Errorf("failed to get image with tag %q: %v", tag, err)
				}
				if got.Digest.String() != d.DigestStr() {
					t.Errorf("image with tag %q has wrong digest: got %s, want %s", tag, got.Digest, d.DigestStr())
				}
			}
		}
	}
	checkTags(map[string][]string{
		dig1.String(): {"1", "foo", "bar", "baz"},
		dig2.String(): {"2", "hello", "world"},
	})

	// Update some tags.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`resource "oci_tags" "test" {
				repo = %q
				tags = %s
			}`, repo, marshal(map[string]v1.Hash{
				// "foo" isn't specified, but this doesn't untag it.
				"bar":     d1,
				"baz":     d1,
				"hello":   d1, // "hello" moved from 2 to 1.
				"world":   d2,
				"goodbye": d1, // new tag on 1.
				"kevin":   d2, // new tag on 2.
			})),
		}},
	})
	checkTags(map[string][]string{
		dig1.String(): {"1", "foo", "bar", "baz", "hello", "goodbye"},
		dig2.String(): {"2", "world", "kevin"},
	})
}
