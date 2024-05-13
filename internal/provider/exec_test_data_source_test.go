package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExecTestDataSource(t *testing.T) {
	img, err := remote.Image(name.MustParseReference("cgr.dev/chainguard/wolfi-base:latest"))
	if err != nil {
		t.Fatalf("failed to fetch image: %v", err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`data "oci_exec_test" "test" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"

  script = "docker run --rm $${IMAGE_NAME} echo hello | grep hello"
}`, d.String()),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.oci_exec_test.test", "digest", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestMatchResourceAttr("data.oci_exec_test.test", "id", regexp.MustCompile(".*cgr.dev/chainguard/wolfi-base@"+d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.test", "exit_code", "0"),
				resource.TestCheckResourceAttr("data.oci_exec_test.test", "output", ""),
			),
		}, {
			Config: fmt.Sprintf(`data "oci_exec_test" "env" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"

  env {
	name  = "FOO"
	value = "bar"
  }
  env {
	name  = "BAR"
	value = "baz"
  }

  script = "echo IMAGE_NAME=$${IMAGE_NAME} IMAGE_REPOSITORY=$${IMAGE_REPOSITORY} IMAGE_REGISTRY=$${IMAGE_REGISTRY} FOO=bar BAR=baz FREE_PORT=$${FREE_PORT}"
}`, d.String()),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "digest", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestMatchResourceAttr("data.oci_exec_test.env", "id", regexp.MustCompile(".*cgr.dev/chainguard/wolfi-base@"+d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "exit_code", "0"),
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "output", ""),
			),
		}, {
			Config: fmt.Sprintf(`data "oci_exec_test" "fail" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"

  script = "echo failed && exit 12"
}`, d.String()),
			ExpectError: regexp.MustCompile(`Test failed for ref\ncgr.dev/chainguard/wolfi-base@sha256:[0-9a-f]{64},\ngot error: exit status 12\nfailed`),
			// We don't get the exit code or output because the datasource failed.
		}, {
			Config: fmt.Sprintf(`data "oci_exec_test" "timeout" {
	  digest = "cgr.dev/chainguard/wolfi-base@%s"
	  timeout_seconds = 1

	  script = "sleep 6"
	}`, d.String()),
			ExpectError: regexp.MustCompile(`Test for ref\ncgr.dev/chainguard/wolfi-base@sha256:[0-9a-f]{64}\ntimed out after 1 seconds`),
		}, {
			Config: fmt.Sprintf(`data "oci_exec_test" "working_dir" {
		  digest = "cgr.dev/chainguard/wolfi-base@%s"
		  working_dir = "${path.module}/../../"

		  script = "grep 'Terraform Provider for OCI operations' README.md"
		}`, d.String()),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "digest", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestMatchResourceAttr("data.oci_exec_test.working_dir", "id", regexp.MustCompile(".*cgr.dev/chainguard/wolfi-base@"+d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "exit_code", "0"),
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "output", ""),
			),
		}},
	})

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"oci": providerserver.NewProtocol6WithError(&OCIProvider{
				defaultExecTimeoutSeconds: 1,
			}),
		}, Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`data "oci_exec_test" "provider-timeout" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"

  script = "sleep 6"
}`, d.String()),
			ExpectError: regexp.MustCompile(`Test for ref\ncgr.dev/chainguard/wolfi-base@sha256:[0-9a-f]{64}\ntimed out after 1 seconds`),
		}},
	})

}

func TestAccExecTestDataSource_FreePort(t *testing.T) {
	img, err := remote.Image(name.MustParseReference("cgr.dev/chainguard/wolfi-base:latest"))
	if err != nil {
		t.Fatalf("failed to fetch image: %v", err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("failed to get image digest: %v", err)
	}

	// Test that we can spin up a bunch of parallel tasks that each get
	// a unique free port, even if they don't run anything on that port.
	cfg := ""
	checks := []resource.TestCheckFunc{}
	num := 10
	for i := 0; i < num; i++ {
		cfg += fmt.Sprintf(`data "oci_exec_test" "freeport-%d" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"
  script = "docker run --rm $${IMAGE_NAME} echo $${FREE_PORT}"
}
`, i, d.String())
		checks = append(checks,
			resource.TestCheckResourceAttr(fmt.Sprintf("data.oci_exec_test.freeport-%d", i), "digest", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
			resource.TestMatchResourceAttr(fmt.Sprintf("data.oci_exec_test.freeport-%d", i), "id", regexp.MustCompile(".*cgr.dev/chainguard/wolfi-base@"+d.String())),
		)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: cfg,
			Check:  resource.ComposeAggregateTestCheckFunc(checks...),
		}},
	})
}
