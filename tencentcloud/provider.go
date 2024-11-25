package tencentcloud

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	tencentCloudCamClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cam/v20190116"
	tencentCloudCdnClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	tencentCloudClbClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
)

// Wrapper of Tencent Cloud client
type tencentCloudClients struct {
	clbClient *tencentCloudClbClient.Client
	camClient *tencentCloudCamClient.Client
	cdnClient *tencentCloudCdnClient.Client
}

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &tencentCloudProvider{}
)

// New is a helper function to simplify provider server
func New() provider.Provider {
	return &tencentCloudProvider{}
}

type tencentCloudProvider struct{}

type tencentCloudProviderModel struct {
	Region    types.String `tfsdk:"region"`
	SecretId  types.String `tfsdk:"secret_id"`
	SecretKey types.String `tfsdk:"secret_key"`
}

// Metadata returns the provider type name.
func (p *tencentCloudProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "st-tencentcloud"
}

// Schema defines the provider-level schema for configuration data.
func (p *tencentCloudProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Tencent Cloud provider is used to interact with the many resources supported by Tencent Cloud. " +
			"The provider needs to be configured with the proper credentials before it can be used.",
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Description: "Region for Tencent Cloud API. May also be provided via TENCENTCLOUD_REGION environment variable.",
				Optional:    true,
			},
			"secret_id": schema.StringAttribute{
				Description: "Access Key for Tencent Cloud API. May also be provided via TENCENTCLOUD_SECRET_ID environment variable",
				Optional:    true,
			},
			"secret_key": schema.StringAttribute{
				Description: "Secret key for Tencent Cloud API. May also be provided via TENCENTCLOUD_SECRET_KEY environment variable",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// Configure prepares a Tencent Cloud API client for data sources and resources.
func (p *tencentCloudProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config tencentCloudProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.
	if config.Region.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("region"),
			"Unknown Tencent Cloud region",
			"The provider cannot create the Tencent Cloud API client as there is an unknown configuration value for the"+
				"Tencent Cloud API region. Set the value statically in the configuration, or use the TENCENTCLOUD_REGION environment variable.",
		)
	}

	if config.SecretId.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_id"),
			"Unknown Tencent Cloud access key",
			"The provider cannot create the Tencent Cloud API client as there is an unknown configuration value for the"+
				"Tencent Cloud API access key. Set the value statically in the configuration, or use the TENCENTCLOUD_SECRET_ID environment variable.",
		)
	}

	if config.SecretKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_key"),
			"Unknown Tencent Cloud secret key",
			"The provider cannot create the Tencent Cloud API client as there is an unknown configuration value for the"+
				"Tencent Cloud secret key. Set the value statically in the configuration, or use the TENCENTCLOUD_SECRET_KEY environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.
	var region, secretId, secretKey string
	if !config.Region.IsNull() {
		region = config.Region.ValueString()
	} else {
		region = os.Getenv("TENCENTCLOUD_REGION")
	}

	if !config.SecretId.IsNull() {
		secretId = config.SecretId.ValueString()
	} else {
		secretId = os.Getenv("TENCENTCLOUD_SECRET_ID")
	}

	if !config.SecretKey.IsNull() {
		secretKey = config.SecretKey.ValueString()
	} else {
		secretKey = os.Getenv("TENCENTCLOUD_SECRET_KEY")
	}

	// If any of the expected configuration are missing, return
	// errors with provider-specific guidance.
	if region == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("region"),
			"Missing Tencent Cloud API region",
			"The provider cannot create the Tencent Cloud API client as there is a "+
				"missing or empty value for the Tencent Cloud API region. Set the "+
				"region value in the configuration or use the TENCENTCLOUD_REGION "+
				"environment variable. If either is already set, ensure the value "+
				"is not empty.",
		)
	}

	if secretId == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_id"),
			"Missing Tencent Cloud API access key",
			"The provider cannot create the Tencent Cloud API client as there is a "+
				"missing or empty value for the Tencent Cloud API access key. Set the "+
				"access key value in the configuration or use the TENCENTCLOUD_SECRET_ID "+
				"environment variable. If either is already set, ensure the value "+
				"is not empty.",
		)
	}

	if secretKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("secret_key"),
			"Missing Tencent Cloud secret key",
			"The provider cannot create the Tencent Cloud API client as there is a "+
				"missing or empty value for the Tencent Cloud API Secret Key. Set the "+
				"secret key value in the configuration or use the TENCENTCLOUD_SECRET_KEY "+
				"environment variable. If either is already set, ensure the value "+
				"is not empty.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	clientCredentialsConfig := common.NewCredential(
		secretId,
		secretKey,
	)

	// Tencent Cloud Load Balancers Client
	// HongKong = "ap-hongkong"
	// https://github.com/TencentCloud/tencentcloud-sdk-go/blob/master/tencentcloud/common/regions/regions.go
	clbClient, err := tencentCloudClbClient.NewClient(clientCredentialsConfig, region, profile.NewClientProfile())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Tencent Cloud Load Balancers API Client",
			"An unexpected error occurred when creating the Tencent Cloud Load Balancers API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Tencent Cloud Load Balancers Client Error: "+err.Error(),
		)
		return
	}

	camClient, err := tencentCloudCamClient.NewClient(clientCredentialsConfig, region, profile.NewClientProfile())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Tencent Cloud CAM API Client",
			"An unexpected error occurred when creating the Tencent Cloud CAM API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Tencent Cloud CAM Client Error: "+err.Error(),
		)
		return
	}

	cdnClient, err := tencentCloudCdnClient.NewClient(clientCredentialsConfig, region, profile.NewClientProfile())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Tencent Cloud CDN API Client",
			"An unexpected error occurred when creating the Tencent Cloud CDN API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Tencent Cloud CDN Client Error: "+err.Error(),
		)
	}

	// Tencent Cloud clients wrapper
	tencentCloudClients := tencentCloudClients{
		clbClient: clbClient,
		camClient: camClient,
		cdnClient: cdnClient,
	}

	resp.DataSourceData = tencentCloudClients
	resp.ResourceData = tencentCloudClients
}

func (p *tencentCloudProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClbInstancesDataSource,
		NewCdnDomainsDataSource,
	}
}

func (p *tencentCloudProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewCamUserGroupAttachmentResource,
		NewCamMfaDeviceResource,
		NewCdnConditionalOriginResource,
	}
}
