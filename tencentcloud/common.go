package tencentcloud

import "github.com/hashicorp/terraform-plugin-framework/types"

type clientConfig struct {
	Region    types.String `tfsdk:"region"`
	SecretId  types.String `tfsdk:"secret_id"`
	SecretKey types.String `tfsdk:"secret_key"`
}

type clientConfigWithZone struct {
	Region    types.String `tfsdk:"region"`
	Zone      types.String `tfsdk:"zone"`
	SecretId  types.String `tfsdk:"secret_id"`
	SecretKey types.String `tfsdk:"secret_key"`
}

func (cfg *clientConfigWithZone) getClientConfig() *clientConfig {
	return &clientConfig{
		Region:    cfg.Region,
		SecretId:  cfg.SecretId,
		SecretKey: cfg.SecretKey,
	}
}
