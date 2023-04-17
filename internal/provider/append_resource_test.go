package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExampleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccExampleResourceConfig("alpine:3.17"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("crane_append.test", "base_image", "alpine:3.17"),
					resource.TestCheckResourceAttr("crane_append.test", "id", "example-id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "crane_append.test",
				ImportState:       true,
				ImportStateVerify: true,
				// This is not normally necessary, but is here because this
				// example code does not have an actual upstream service.
				// Once the Read method is able to refresh information from
				// the upstream service, this can be removed.
				ImportStateVerifyIgnore: []string{"base_image"},
			},
			// Update and Read testing
			{
				Config: testAccExampleResourceConfig("alpine:3.18"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("crane_append.test", "base_image", "alpine:3.18"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccExampleResourceConfig(baseImage string) string {
	return fmt.Sprintf(`
resource "crane_append" "test" {
  base_image = %[1]q
}
`, baseImage)
}
