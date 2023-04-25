package provider

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TODO: move this into a shared testing library where tf-{ko,apko,cosign} can use it too.
func setupRegistry(t *testing.T) (name.Registry, func()) {
	t.Helper()
	if got := os.Getenv("TF_OCI_REGISTRY"); got != "" {
		reg, err := name.NewRegistry(got)
		if err != nil {
			t.Fatalf("failed to parse TF_OCI_REGISTRY: %v", err)
		}
		return reg, func() {}
	}
	srv := httptest.NewServer(registry.New())
	t.Logf("Started registry: %s", srv.URL)
	reg, err := name.NewRegistry(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("failed to parse TF_OCI_REGISTRY: %v", err)
	}
	return reg, srv.Close
}

func TestAccExampleDataSource(t *testing.T) {
	reg, cleanup := setupRegistry(t)
	defer cleanup()

	ref, err := reg.Repository("test").Tag("1")
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
			},
		},
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// A ref specified by digest has no .tag attribute.
			{
				Config: fmt.Sprintf(`data "crane_ref" "test" {
				  ref = %q
				}`, refByDigest),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.crane_ref.test", "id", ref.Context().Digest(d.String()).String()),
					resource.TestCheckResourceAttr("data.crane_ref.test", "digest", d.String()),
					resource.TestCheckNoResourceAttr("data.crane_ref.test", "tag"),
				),
			},
		},
	})
}
