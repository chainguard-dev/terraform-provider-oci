package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// pushRandomImage writes a random image to repo:tag and returns its digest.
func pushRandomImage(t *testing.T, repo name.Repository, tag string) v1.Hash {
	t.Helper()
	img, err := random.Image(1024, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	if err := remote.Write(repo.Tag(tag), img); err != nil {
		t.Fatalf("remote.Write: %v", err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("img.Digest: %v", err)
	}
	return d
}

// pushRandomIndex writes a two-platform random index to repo:tag and returns
// its digest.
func pushRandomIndex(t *testing.T, repo name.Repository, tag string) v1.Hash {
	t.Helper()
	var idx v1.ImageIndex = empty.Index
	for _, plat := range []v1.Platform{{OS: "linux", Architecture: "amd64"}, {OS: "linux", Architecture: "arm64"}} {
		img, err := random.Image(1024, 1)
		if err != nil {
			t.Fatalf("random.Image: %v", err)
		}
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{Add: img, Platform: &plat})
	}
	if err := remote.WriteIndex(repo.Tag(tag), idx); err != nil {
		t.Fatalf("remote.WriteIndex: %v", err)
	}
	d, err := idx.Digest()
	if err != nil {
		t.Fatalf("idx.Digest: %v", err)
	}
	return d
}

func TestAccRefDataSource(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	imgDigest := pushRandomImage(t, repo, "image")
	idxDigest := pushRandomIndex(t, repo, "index")

	for _, tc := range []struct {
		name   string
		ref    string
		checks []statecheck.StateCheck
		err    *regexp.Regexp
	}{{
		name: "image by tag",
		ref:  repo.Tag("image").String(),
		checks: []statecheck.StateCheck{
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("digest"), knownvalue.StringExact(imgDigest.String())),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("full_ref"), knownvalue.StringExact(repo.Digest(imgDigest.String()).String())),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("tag"), knownvalue.StringExact("image")),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("config"), knownvalue.NotNull()),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("images"), knownvalue.Null()),
		},
	}, {
		name: "index by tag",
		ref:  repo.Tag("index").String(),
		checks: []statecheck.StateCheck{
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("digest"), knownvalue.StringExact(idxDigest.String())),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("tag"), knownvalue.StringExact("index")),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("config"), knownvalue.Null()),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("images").AtMapKey("linux/arm64").AtMapKey("digest"), knownvalue.StringRegexp(digestRE)),
		},
	}, {
		name: "image by digest has no tag",
		ref:  repo.Digest(imgDigest.String()).String(),
		checks: []statecheck.StateCheck{
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("digest"), knownvalue.StringExact(imgDigest.String())),
			statecheck.ExpectKnownValue("data.oci_ref.test", tfjsonpath.New("tag"), knownvalue.StringExact("")),
		},
	}, {
		name: "missing tag fails",
		ref:  repo.Tag("missing").String(),
		err:  regexp.MustCompile(`Unable to fetch image`),
	}, {
		name: "unparseable ref fails",
		ref:  "not a ref",
		err:  regexp.MustCompile(`Invalid image reference`),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:            fmt.Sprintf(`data "oci_ref" "test" { ref = %q }`, tc.ref),
					ConfigStateChecks: tc.checks,
					ExpectError:       tc.err,
				}},
			})
		})
	}
}

func TestAccRefDataSource_ValidatesRef(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			// With count = 0 Terraform never reads the data source, so only
			// validation can reject the ref.
			Config: `
data "oci_ref" "test" {
  count = 0
  ref   = "not a ref"
}
`,
			ExpectError: regexp.MustCompile(`Invalid image reference`),
		}},
	})
}

func TestAccRefDataSource_MatchesGetFunction(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	imgDigest := pushRandomImage(t, repo, "image")
	pushRandomIndex(t, repo, "index")

	for _, tc := range []struct {
		name string
		ref  string
	}{{
		name: "image by tag",
		ref:  repo.Tag("image").String(),
	}, {
		name: "index by tag",
		ref:  repo.Tag("index").String(),
	}, {
		name: "image by digest",
		ref:  repo.Digest(imgDigest.String()).String(),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
				Steps: []resource.TestStep{{
					// Terraform compares function results as whole objects, so
					// compare the whole object here too.
					Config: fmt.Sprintf(`
data "oci_ref" "test" {
  ref = %[1]q
}

output "same" {
  value = provider::oci::get(%[1]q) == {
    full_ref = data.oci_ref.test.full_ref
    digest   = data.oci_ref.test.digest
    tag      = data.oci_ref.test.tag
    manifest = data.oci_ref.test.manifest
    images   = data.oci_ref.test.images
    config   = data.oci_ref.test.config
  }
}
`, tc.ref),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownOutputValue("same", knownvalue.Bool(true)),
					},
				}},
			})
		})
	}
}

// TestGetFunction_Errors pins the function's error messages, which come from
// the fetch-and-describe helper it shares with oci_ref.
func TestGetFunction_Errors(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	for _, tc := range []struct {
		name string
		ref  string
		err  *regexp.Regexp
	}{{
		name: "missing tag",
		ref:  repo.Tag("missing").String(),
		err:  regexp.MustCompile(`Failed\s+to\s+get\s+image:`),
	}, {
		name: "unparseable ref",
		ref:  "not a ref",
		err:  regexp.MustCompile(`Failed\s+to\s+parse\s+OCI\s+reference:`),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
				Steps: []resource.TestStep{{
					Config:      fmt.Sprintf(`output "gotten" { value = provider::oci::get(%q) }`, tc.ref),
					ExpectError: tc.err,
				}},
			})
		})
	}
}

// moveTag is a plan check that pushes img to tag after Terraform plans and
// before it applies, as another push that moves the tag between plan and
// apply does.
type moveTag struct {
	tag name.Tag
	img v1.Image
}

func (m moveTag) CheckPlan(ctx context.Context, _ plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	resp.Error = remote.Write(m.tag, m.img, remote.WithContext(ctx))
}

func TestAccRefDataSource_TagMovesDuringApply(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	tag := repo.Tag("moving")

	for _, tc := range []struct {
		name   string
		lookup string // declares the lookup
		digest string // reads its digest
		err    *regexp.Regexp
	}{{
		name:   "data source keeps the plan-time digest",
		lookup: fmt.Sprintf(`data "oci_ref" "moving" { ref = %q }`, tag),
		digest: "data.oci_ref.moving.digest",
	}, {
		// The failure the data source exists to avoid.
		name:   "get function rejects the moved tag",
		lookup: fmt.Sprintf(`locals { moving = provider::oci::get(%q) }`, tag),
		digest: "local.moving.digest",
		err:    regexp.MustCompile(`inconsistent result`),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			planned := pushRandomImage(t, repo, "moving")
			next, err := random.Image(1024, 1)
			if err != nil {
				t.Fatalf("random.Image: %v", err)
			}

			var checks []statecheck.StateCheck
			if tc.err == nil {
				checks = []statecheck.StateCheck{
					statecheck.ExpectKnownValue("terraform_data.consumer", tfjsonpath.New("output"), knownvalue.StringExact(planned.String())),
				}
			}

			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
				Steps: []resource.TestStep{{
					// Terraform evaluates a resource's arguments again during
					// apply; it does not for root outputs.
					Config: tc.lookup + fmt.Sprintf(`
resource "terraform_data" "consumer" {
  input = %s
}
`, tc.digest),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{moveTag{tag: tag, img: next}},
					},
					ConfigStateChecks: checks,
					ExpectError:       tc.err,
					// The refresh after apply sees the moved tag.
					ExpectNonEmptyPlan: true,
				}},
			})
		})
	}
}
