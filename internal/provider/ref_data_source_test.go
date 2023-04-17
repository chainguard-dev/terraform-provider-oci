package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccExampleDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccExampleDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.crane_ref.test", "id", "alpine@sha256:124c7d2707904eea7431fffe91522a01e5a861a624ee31d03372cc1d138a3126"),
					resource.TestCheckResourceAttr("data.crane_ref.test", "digest", "sha256:124c7d2707904eea7431fffe91522a01e5a861a624ee31d03372cc1d138a3126"),
				),
			},
		},
	})
}

const testAccExampleDataSourceConfig = `
data "crane_ref" "test" {
  ref = "alpine@sha256:124c7d2707904eea7431fffe91522a01e5a861a624ee31d03372cc1d138a3126"
}
`
