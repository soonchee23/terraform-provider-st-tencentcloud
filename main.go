package main

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/myklst/terraform-provider-st-tencentcloud/tencentcloud"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name st-tencentcloud

func main() {
	providerAddress := os.Getenv("PROVIDER_LOCAL_PATH")
	if providerAddress == "" {
		providerAddress = "registry.terraform.io/myklst/st-tencentcloud"
	}
	providerserver.Serve(context.Background(), tencentcloud.New, providerserver.ServeOpts{
		Address: providerAddress,
	})
}
