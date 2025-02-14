package tencentcloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tencentCloudCdnClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

var (
	_ resource.Resource              = &cdnPathBasedRuleResource{}
	_ resource.ResourceWithConfigure = &cdnPathBasedRuleResource{}
)

func NewCdnPathBasedRuleResource() resource.Resource {
	return &cdnPathBasedRuleResource{}
}

type cdnPathBasedRuleResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnPathBasedRuleResourceModel struct {
	DomainName types.String `tfsdk:"domain"`
	Origin     []*origin    `tfsdk:"origin"`
}

type origin struct {
	Origins       types.List       `tfsdk:"origin_list"`
	OriginType    types.String     `tfsdk:"origin_type"`
	ServerName    types.String     `tfsdk:"server_name"`
	PathBasedRule []*pathBasedRule `tfsdk:"path_based_rule"`
}

type pathBasedRule struct {
	RuleType  types.String `tfsdk:"rule_type"`
	RulePaths types.List   `tfsdk:"rule_paths"`
	Origin    types.List   `tfsdk:"origin"`
}

func (r *cdnPathBasedRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_path_based_rule"
}

func (r *cdnPathBasedRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a TencentCloud Path-Based Rule resource.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"origin": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"origin_list": schema.ListAttribute{
							ElementType: types.StringType,
							Description: "List of rule paths for origin.",
							Optional:    true,
						},
						"origin_type": schema.StringAttribute{
							Description: "Domain name.",
							Optional:    true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									"domain",
									"domainv6",
									"cos",
									"third_party",
									"igtm",
									"ip",
									"ipv6",
									"ip_ipv6",
									"ip_domain",
									"ip_domainv6",
									"ipv6_domain",
									"ipv6_domainv6",
									"domain_domainv6",
									"ip_ipv6_domain",
									"ip_ipv6_domainv6",
									"ip_domain_domainv6",
									"ipv6_domain_domainv6",
									"ip_ipv6_domain_domainv6",
									"image",
									"ftp"),
							},
						},
						"server_name": schema.StringAttribute{
							Description: "Server name.",
							Optional:    true,
						},
					},
					Blocks: map[string]schema.Block{
						"path_based_rule": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"origin": schema.ListAttribute{
										ElementType: types.StringType,
										Description: "List of origin servers.",
										Required:    true,
									},
									"rule_paths": schema.ListAttribute{
										ElementType: types.StringType,
										Description: "List of rule paths for origin.",
										Required:    true,
									},
									"rule_type": schema.StringAttribute{
										Description: "Type of the rule for origin.",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf("file", "directory", "path", "index"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *cdnPathBasedRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (r *cdnPathBasedRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cdnPathBasedRuleResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.updateDomainConfig(&plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update domain config",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnPathBasedRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *cdnPathBasedRuleResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeDomainsConfigRequest := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	if !(state.DomainName.IsUnknown() || state.DomainName.IsNull()) {
		describeDomainsConfigRequest.Filters = []*tencentCloudCdnClient.DomainFilter{
			{
				Name:  common.StringPtr("domain"),
				Value: common.StringPtrs([]string{state.DomainName.String()}),
			},
		}
	}

	describeDomainsConfig := func() error {
		_, err := r.client.DescribeDomainsConfig(describeDomainsConfigRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isRetryableErrCode(t.GetCode()) {
					return err
				} else {
					return backoff.Permanent(err)
				}
			} else {
				return err
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(describeDomainsConfig, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe CDN.",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnPathBasedRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *cdnPathBasedRuleResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.updateDomainConfig(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update CDN configuration.",
			fmt.Sprintf("Error: %s. Request payload: %+v", err.Error(), plan),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnPathBasedRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *cdnPathBasedRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	/*The reason for using UpdateDomainConfig is that Tencent Cloud only provides this API to remove
	path-based rules; they have a separate DeleteScdnDomain function exclusively for deleting an entire
	domain name.*/
	deleteDomainConfigRequest, err := buildUpdateDomainConfigRequest(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to build CDN domain config",
			err.Error(),
		)
		return
	}

	deleteDomainConfigRequest.Origin.PathBasedOrigin = nil
	if _, err := r.client.UpdateDomainConfig(deleteDomainConfigRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CDN Domain",
			err.Error(),
		)
		return
	}

	err = waitForCDNDomainStatus(r.client, state.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"[TIMEOUT] Timed out waiting for domain status to become online",
			err.Error(),
		)
		return
	}
}

func (d *cdnPathBasedRuleResource) updateDomainConfig(plan *cdnPathBasedRuleResourceModel) error {
	updateDomainConfigRequest, err := buildUpdateDomainConfigRequest(plan)
	if err != nil {
		return fmt.Errorf("failed to build domain config: %w", err)
	}

	updateDomainConfig := func() error {
		_, err := d.client.UpdateDomainConfig(updateDomainConfigRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok && isRetryableErrCode(t.GetCode()) {
				return err
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(updateDomainConfig, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to update domain config: %w", err)
	}

	err = waitForCDNDomainStatus(d.client, plan.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		return err
	}

	return nil
}

func buildUpdateDomainConfigRequest(plan *cdnPathBasedRuleResourceModel) (*tencentCloudCdnClient.UpdateDomainConfigRequest, error) {
	if plan.DomainName.ValueString() == "" {
		return nil, fmt.Errorf("domain name cannot be empty")
	}

	updateDomainConfigRequest := tencentCloudCdnClient.NewUpdateDomainConfigRequest()
	updateDomainConfigRequest.Domain = common.StringPtr(plan.DomainName.ValueString())

	for _, origin := range plan.Origin {
		mainOrigins := make([]string, len(origin.Origins.Elements()))
		for i, o := range origin.Origins.Elements() {
			mainOrigins[i] = strings.Trim(o.(types.String).ValueString(), "\"")
		}

		var pathBasedOriginRules []*tencentCloudCdnClient.PathBasedOriginRule
		for _, pathBasedRule := range origin.PathBasedRule {
			rulePaths := make([]string, len(pathBasedRule.RulePaths.Elements()))
			for i, rp := range pathBasedRule.RulePaths.Elements() {
				rulePaths[i] = strings.Trim(rp.(types.String).ValueString(), "\"")
			}

			pathOrigins := make([]string, len(pathBasedRule.Origin.Elements()))
			for i, o := range pathBasedRule.Origin.Elements() {
				pathOrigins[i] = strings.Trim(o.(types.String).ValueString(), "\"")
			}

			if len(rulePaths) == 0 || len(pathOrigins) == 0 {
				return nil, fmt.Errorf("both rule paths and origins must be provided")
			}

			pathBasedOriginRules = append(pathBasedOriginRules, &tencentCloudCdnClient.PathBasedOriginRule{
				RuleType:  common.StringPtr(pathBasedRule.RuleType.ValueString()),
				RulePaths: common.StringPtrs(rulePaths),
				Origin:    common.StringPtrs(pathOrigins),
			})
		}

		/*The Origin List, Origin Type, and Server Name need to be specified again, as they are part of the
		tencentcloud_cdn_domains resource in the Tencent Cloud Terraform provider. This is necessary because
		the path-based rule is nested within the origin configuration, and it's important to associate the
		path-based rule with the correct origin.*/
		updateDomainConfigRequest.Origin = &tencentCloudCdnClient.Origin{
			Origins:    common.StringPtrs(mainOrigins),
			OriginType: common.StringPtr(origin.OriginType.ValueString()),
			ServerName: common.StringPtr(func() string {
				if origin.ServerName.ValueString() == "" {
					return plan.DomainName.ValueString()
				}
				return origin.ServerName.ValueString()
			}()),
			PathBasedOrigin: pathBasedOriginRules,
		}
	}

	updateDomainConfigRequest.ProjectId = common.Int64Ptr(0)
	return updateDomainConfigRequest, nil
}

func waitForCDNDomainStatus(client *tencentCloudCdnClient.Client, domainName string, timeout time.Duration) error {
	decribeDomainsConfig := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	decribeDomainsConfig.Filters = []*tencentCloudCdnClient.DomainFilter{
		{
			Name:  common.StringPtr("domain"),
			Value: common.StringPtrs([]string{domainName}),
		},
	}

	checkStatus := func() (bool, error) {
		response, err := client.DescribeDomainsConfig(decribeDomainsConfig)
		if err != nil {
			return false, err
		}

		if len(response.Response.Domains) > 0 {
			status := response.Response.Domains[0].Status
			if status != nil && *status == "online" {
				return true, nil
			}
		}
		return false, nil
	}

	startTime := time.Now()
	for {
		isOnline, err := checkStatus()
		if err != nil {
			return fmt.Errorf("error checking CDN domain status: %w", err)
		}

		if isOnline {
			break
		}

		if time.Since(startTime) > timeout {
			return fmt.Errorf("timed out waiting for domain status to become online")
		}

		time.Sleep(30 * time.Second)
	}

	return nil
}
