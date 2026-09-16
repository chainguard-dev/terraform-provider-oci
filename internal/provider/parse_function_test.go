package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

var digestRE = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

func TestParseFunction(t *testing.T) {
	// A naked ref string errors due to missing digest
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config:      `output "parsed" { value = provider::oci::parse("") }`,
			ExpectError: regexp.MustCompile(""), // any error is ok
		}},
	})

	// A fully qualified tag ref string errors due to missing digest
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config:      `output "parsed" { value = provider::oci::parse("cgr.dev/foo/sample:latest") }`,
			ExpectError: regexp.MustCompile(""), // any error is ok
		}},
	})

	// A fully qualified ref
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("cgr.dev/foo/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("cgr.dev"),
					"repo":                 knownvalue.StringExact("foo/sample"),
					"registry_repo":        knownvalue.StringExact("cgr.dev/foo/sample"),
					"registry_repo_prefix": knownvalue.StringExact("cgr.dev/foo"),
					"image_name":           knownvalue.StringExact("sample"),
					"digest":               knownvalue.StringExact("sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"ref":                  knownvalue.StringExact("cgr.dev/foo/sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				})),
			},
		}},
	})

	// A shorthand digest ref string has everything (including a pseudo tag)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("sample@sha256:1234567890123456789012345678901234567890123456789012345678901234") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("index.docker.io"),
					"repo":                 knownvalue.StringExact("library/sample"),
					"registry_repo":        knownvalue.StringExact("index.docker.io/library/sample"),
					"registry_repo_prefix": knownvalue.StringExact("index.docker.io/library"),
					"image_name":           knownvalue.StringExact("sample"),
					"digest":               knownvalue.StringExact("sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"ref":                  knownvalue.StringExact("sample@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				})),
			},
		}},
	})

	// A shorthand tagged and digest ref string has everything (including a replaced pseudo tag)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("sample:cursed@sha256:1234567890123456789012345678901234567890123456789012345678901234") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("index.docker.io"),
					"repo":                 knownvalue.StringExact("library/sample"),
					"registry_repo":        knownvalue.StringExact("index.docker.io/library/sample"),
					"registry_repo_prefix": knownvalue.StringExact("index.docker.io/library"),
					"image_name":           knownvalue.StringExact("sample"),
					"digest":               knownvalue.StringExact("sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
					"ref":                  knownvalue.StringExact("sample:cursed@sha256:1234567890123456789012345678901234567890123456789012345678901234"),
				})),
			},
		}},
	})

	// A cgr.dev reference splits image_name from a two-segment repo
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("cgr.dev/chainguard/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("cgr.dev"),
					"repo":                 knownvalue.StringExact("chainguard/nginx"),
					"registry_repo":        knownvalue.StringExact("cgr.dev/chainguard/nginx"),
					"registry_repo_prefix": knownvalue.StringExact("cgr.dev/chainguard"),
					"image_name":           knownvalue.StringExact("nginx"),
					"digest":               knownvalue.StringExact("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
					"ref":                  knownvalue.StringExact("cgr.dev/chainguard/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
				})),
			},
		}},
	})

	// A private chainguard reference keeps the full org segment in the prefix
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("cgr.dev/chainguard-private/istio-pilot@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("cgr.dev"),
					"repo":                 knownvalue.StringExact("chainguard-private/istio-pilot"),
					"registry_repo":        knownvalue.StringExact("cgr.dev/chainguard-private/istio-pilot"),
					"registry_repo_prefix": knownvalue.StringExact("cgr.dev/chainguard-private"),
					"image_name":           knownvalue.StringExact("istio-pilot"),
					"digest":               knownvalue.StringExact("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
					"ref":                  knownvalue.StringExact("cgr.dev/chainguard-private/istio-pilot@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				})),
			},
		}},
	})

	// A single-segment repo leaves the registry as the whole prefix
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: `output "parsed" { value = provider::oci::parse("cgr.dev/root@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc") }`,
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("parsed", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"registry":             knownvalue.StringExact("cgr.dev"),
					"repo":                 knownvalue.StringExact("root"),
					"registry_repo":        knownvalue.StringExact("cgr.dev/root"),
					"registry_repo_prefix": knownvalue.StringExact("cgr.dev"),
					"image_name":           knownvalue.StringExact("root"),
					"digest":               knownvalue.StringExact("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
					"pseudo_tag":           knownvalue.StringExact("unused@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
					"ref":                  knownvalue.StringExact("cgr.dev/root@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
				})),
			},
		}},
	})
}
