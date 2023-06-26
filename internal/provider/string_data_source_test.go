package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccStringDataSource(t *testing.T) {
	// A naked ref string errors due to missing digest
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(`data "oci_string" "test" { input = %q }`, "sample"),
				ExpectError: regexp.MustCompile(""), // any error is ok
			},
		},
	})

	// A fully qualified tag ref string errors due to missing digest
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(`data "oci_string" "test" { input = %q }`, "cgr.dev/foo/sample:latest"),
				ExpectError: regexp.MustCompile(""), // any error is ok
			},
		},
	})

	// A fully qualified ref
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_string" "test" { input = %q }`, "cgr.dev/foo/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_string.test", "id", "cgr.dev/foo/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry", "cgr.dev"),
					resource.TestCheckResourceAttr("data.oci_string.test", "repo", "foo/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry_repo", "cgr.dev/foo/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "pseudo_tag", "unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "digest", "sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				),
			},
		},
	})

	// A shorthand digest ref string has everything (including a pseudo tag)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_string" "test" { input = %q }`, "sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_string.test", "id", "index.docker.io/library/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry", "index.docker.io"),
					resource.TestCheckResourceAttr("data.oci_string.test", "repo", "library/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry_repo", "index.docker.io/library/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "pseudo_tag", "unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "digest", "sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				),
			},
		},
	})

	// A shorthand tagged and digest ref string has everything (including a replaced pseudo tag)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "oci_string" "test" { input = %q }`, "sample:cursed@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.oci_string.test", "id", "index.docker.io/library/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry", "index.docker.io"),
					resource.TestCheckResourceAttr("data.oci_string.test", "repo", "library/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "registry_repo", "index.docker.io/library/sample"),
					resource.TestCheckResourceAttr("data.oci_string.test", "pseudo_tag", "unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					resource.TestCheckResourceAttr("data.oci_string.test", "digest", "sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				),
			},
		},
	})
}
