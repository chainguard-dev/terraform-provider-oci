package main

import (
	"context"
	"flag"
	"log"

	"github.com/chainguard-dev/terraform-provider-oci/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

//go:generate terraform fmt -recursive ./examples/.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

const version string = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/chainguard-dev/oci",
		Debug:   debug,
	})

	if err != nil {
		log.Fatal(err.Error())
	}
}
