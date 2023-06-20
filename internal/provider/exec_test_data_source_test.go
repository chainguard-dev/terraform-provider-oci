package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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
				resource.TestCheckResourceAttr("data.oci_exec_test.test", "id", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.test", "exit_code", "0"),
				resource.TestMatchResourceAttr("data.oci_exec_test.test", "output", regexp.MustCompile("hello\n")),
			),
		}, {
			Config: fmt.Sprintf(`data "oci_exec_test" "env" {
  digest = "cgr.dev/chainguard/wolfi-base@%s"

  script = "echo IMAGE_NAME=$${IMAGE_NAME} IMAGE_REPOSITORY=$${IMAGE_REPOSITORY} IMAGE_REGISTRY=$${IMAGE_REGISTRY}"
}`, d.String()),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "digest", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "id", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "exit_code", "0"),
				resource.TestCheckResourceAttr("data.oci_exec_test.env", "output", fmt.Sprintf("IMAGE_NAME=cgr.dev/chainguard/wolfi-base@%s IMAGE_REPOSITORY=chainguard/wolfi-base IMAGE_REGISTRY=cgr.dev\n", d.String())),
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
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "id", fmt.Sprintf("cgr.dev/chainguard/wolfi-base@%s", d.String())),
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "exit_code", "0"),
				resource.TestCheckResourceAttr("data.oci_exec_test.working_dir", "output", "# Terraform Provider for OCI operations\n"),
			),
		}},
	})

}
