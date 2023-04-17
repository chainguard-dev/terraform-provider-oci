package provider

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExampleDataSource(t *testing.T) {
	// Setup a local registry and have tests push to that.
	srv := httptest.NewServer(registry.New())
	defer srv.Close()
	ref, err := name.ParseReference(strings.TrimPrefix(srv.URL, "http://") + "/test:latest")
	if err != nil {
		t.Fatalf("failed to parse reference: %v", err)
	}
	t.Logf("Using ref: %s", ref)

	// Push an image to the local registry.
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

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: fmt.Sprintf(`data "crane_ref" "test" {
				  ref = %q
				}`, ref),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.crane_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.crane_ref.test", "digest", d.String()),
				),
			},
		},
	})
}
