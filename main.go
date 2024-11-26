package main

import (
	"context"
	"flag"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/myklst/terraform-provider-st-tencentcloud/tencentcloud"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name st-tencentcloud

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()
	providerAddress := os.Getenv("PROVIDER_LOCAL_PATH")
	if providerAddress == "" {
		providerAddress = "registry.terraform.io/myklst/st-tencentcloud"
	}
	providerserver.Serve(context.Background(), tencentcloud.New, providerserver.ServeOpts{
		Address: providerAddress,
		Debug:   debug,
	})
}
