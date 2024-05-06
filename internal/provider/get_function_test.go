package provider

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	ocitesting "github.com/chainguard-dev/terraform-provider-oci/testing"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestGetFunction(t *testing.T) {
	repo, cleanup := ocitesting.SetupRepository(t, "test")
	defer cleanup()

	// Push an image to the local registry.
	ref := repo.Tag("latest")
	t.Logf("Using ref: %s", ref)
	img, err := random.Image(1024, 3)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	img = mutate.MediaType(img, ggcrtypes.OCIManifestSchema1)
	img = mutate.Annotations(img, map[string]string{ //nolint:forcetypeassert
		"foo": "bar",
	}).(v1.Image)
	img, err = mutate.Config(img, v1.Config{
		Env:        []string{"FOO=BAR"},
		User:       "nobody",
		Entrypoint: []string{"/bin/sh"},
		Cmd:        []string{"-c", "echo hello world"},
		WorkingDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("failed to mutate image: %v", err)
	}
	now := time.Now()
	img, err = mutate.CreatedAt(img, v1.Time{Time: now})
	if err != nil {
		t.Fatalf("failed to mutate image: %v", err)
	}
	if err := remote.Write(ref, img); err != nil {
		t.Fatalf("failed to write image: %v", err)
	}

	d, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}

	isDescriptor := func(mt ggcrtypes.MediaType) knownvalue.Check {
		return knownvalue.ObjectExact(map[string]knownvalue.Check{
			"digest":     knownvalue.StringRegexp(digestRE),
			"media_type": knownvalue.StringExact(string(mt)),
			"size":       knownvalue.NotNull(),
			"platform":   knownvalue.Null(),
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`output "gotten" { value = provider::oci::get(%q) }`, ref),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("gotten", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"digest": knownvalue.StringExact(d.String()),
					"tag":    knownvalue.StringExact("latest"),
					"manifest": knownvalue.ObjectExact(map[string]knownvalue.Check{
						"schema_version": knownvalue.NumberExact(big.NewFloat(2)),
						"media_type":     knownvalue.StringExact(string(ggcrtypes.OCIManifestSchema1)),
						"config":         isDescriptor(ggcrtypes.DockerConfigJSON),
						"layers": knownvalue.ListExact([]knownvalue.Check{
							isDescriptor(ggcrtypes.DockerLayer),
							isDescriptor(ggcrtypes.DockerLayer),
							isDescriptor(ggcrtypes.DockerLayer),
						}),
						"annotations": knownvalue.MapExact(map[string]knownvalue.Check{"foo": knownvalue.StringExact("bar")}),
						"manifests":   knownvalue.Null(),
						"subject":     knownvalue.Null(),
					}),
					"config": knownvalue.ObjectExact(map[string]knownvalue.Check{
						"env":         knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("FOO=BAR")}),
						"user":        knownvalue.StringExact("nobody"),
						"entrypoint":  knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("/bin/sh")}),
						"cmd":         knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("-c"), knownvalue.StringExact("echo hello world")}),
						"working_dir": knownvalue.StringExact("/tmp"),
						"created_at":  knownvalue.StringExact(now.Format(time.RFC3339)),
					}),
					"images": knownvalue.Null(),
				})),
			},
		}},
	})

	// Push an index to the local registry.
	var idx v1.ImageIndex = empty.Index
	for _, plat := range []v1.Platform{
		{OS: "linux", Architecture: "amd64"},
		{OS: "windows", Architecture: "arm64", Variant: "v3", OSVersion: "1-rc365"},
	} {
		plat := plat
		img, err := random.Image(1024, 3)
		if err != nil {
			t.Fatalf("failed to create image: %v", err)
		}
		img = mutate.MediaType(img, ggcrtypes.OCIManifestSchema1)
		idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
			Add:        img,
			Descriptor: v1.Descriptor{Platform: &plat},
		})
	}
	idx = mutate.IndexMediaType(idx, ggcrtypes.OCIImageIndex)
	idx = mutate.Annotations(idx, map[string]string{ //nolint:forcetypeassert
		"foo": "bar",
	}).(v1.ImageIndex)

	ref = repo.Tag("index")
	t.Logf("Using ref: %s", ref)
	if err := remote.WriteIndex(ref, idx); err != nil {
		t.Fatalf("failed to write index: %v", err)
	}

	d, err = idx.Digest()
	if err != nil {
		t.Fatalf("failed to get index digest: %v", err)
	}

	// An index specified by tag has a .tag attribute, and all the other index manifest attributes.
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_8_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`output "gotten" { value = provider::oci::get(%q) }`, ref),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownOutputValue("gotten", knownvalue.ObjectExact(map[string]knownvalue.Check{
					"digest": knownvalue.StringExact(d.String()),
					"tag":    knownvalue.StringExact("index"),
					"manifest": knownvalue.ObjectExact(map[string]knownvalue.Check{
						"schema_version": knownvalue.NumberExact(big.NewFloat(2)),
						"media_type":     knownvalue.StringExact(string(ggcrtypes.OCIImageIndex)),
						"manifests": knownvalue.ListExact([]knownvalue.Check{
							knownvalue.ObjectExact(map[string]knownvalue.Check{
								"digest": knownvalue.StringRegexp(digestRE),
								"platform": knownvalue.ObjectExact(map[string]knownvalue.Check{
									"os":           knownvalue.StringExact("linux"),
									"architecture": knownvalue.StringExact("amd64"),
									"variant":      knownvalue.StringExact(""),
									"os_version":   knownvalue.StringExact(""),
								}),
								"media_type": knownvalue.StringExact(string(ggcrtypes.OCIManifestSchema1)),
								"size":       knownvalue.NotNull(),
							}),
							knownvalue.ObjectExact(map[string]knownvalue.Check{
								"digest": knownvalue.StringRegexp(digestRE),
								"platform": knownvalue.ObjectExact(map[string]knownvalue.Check{
									"os":           knownvalue.StringExact("windows"),
									"architecture": knownvalue.StringExact("arm64"),
									"variant":      knownvalue.StringExact("v3"),
									"os_version":   knownvalue.StringExact("1-rc365"),
								}),
								"media_type": knownvalue.StringExact(string(ggcrtypes.OCIManifestSchema1)),
								"size":       knownvalue.NotNull(),
							}),
						}),
						"annotations": knownvalue.MapExact(map[string]knownvalue.Check{"foo": knownvalue.StringExact("bar")}),
						"layers":      knownvalue.Null(),
						"subject":     knownvalue.Null(),
						"config":      knownvalue.Null(),
					}),
					"config": knownvalue.Null(),
					"images": knownvalue.MapExact(map[string]knownvalue.Check{
						"linux/amd64": knownvalue.ObjectExact(map[string]knownvalue.Check{
							"digest":    knownvalue.StringRegexp(digestRE),
							"image_ref": knownvalue.NotNull(),
						}),
						"windows/arm64/v3:1-rc365": knownvalue.ObjectExact(map[string]knownvalue.Check{
							"digest":    knownvalue.StringRegexp(digestRE),
							"image_ref": knownvalue.NotNull(),
						}),
					}),
				})),
			},
		}},
	})
}
