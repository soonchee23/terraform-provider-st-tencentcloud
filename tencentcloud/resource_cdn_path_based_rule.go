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
	Origin     []*originn   `tfsdk:"origin"`
}

type originn struct {
	Origins       types.List       `tfsdk:"origin_list"`
	OriginType    types.String     `tfsdk:"origin_type"`
	ServerName    types.String     `tfsdk:"server_name"`
	PathBasedrule []*pathbasedrule `tfsdk:"path_based_rule"`
}

type pathbasedrule struct {
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
						},
						"server_name": schema.StringAttribute{
							Description: "Server name.",
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

	state := &cdnPathBasedRuleResourceModel{
		DomainName: plan.DomainName,
		Origin:     plan.Origin,
	}

	setStateDiags := resp.State.Set(ctx, &state)
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

	describeCdnDataRequest := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()

	if !(state.DomainName.IsUnknown() || state.DomainName.IsNull()) {
		describeCdnDataRequest.Filters = []*tencentCloudCdnClient.DomainFilter{
			{
				Name:  common.StringPtr("domain"),
				Value: common.StringPtrs([]string{state.DomainName.String()}),
			},
		}
	}

	describeCdnData := func() error {
		_, err := r.client.DescribeDomainsConfig(describeCdnDataRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
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
	err := backoff.Retry(describeCdnData, reconnectBackoff)
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

func (r *cdnPathBasedRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *cdnPathBasedRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	origin := state.Origin[0]
	outerOrigins := make([]string, len(origin.Origins.Elements()))

	for i, o := range origin.Origins.Elements() {
		originStr := o.(types.String)
		outerOrigins[i] = strings.Trim(originStr.ValueString(), "\"")
	}

	deleteDomainConfigRequest := tencentCloudCdnClient.NewUpdateDomainConfigRequest()
	deleteDomainConfigRequest.Domain = common.StringPtr(state.DomainName.ValueString())
	deleteDomainConfigRequest.Origin = &tencentCloudCdnClient.Origin{
		Origins:    common.StringPtrs(outerOrigins),
		OriginType: common.StringPtr(origin.OriginType.ValueString()),
		ServerName: common.StringPtr(func() string {
			if origin.ServerName.ValueString() == "" {
				return state.DomainName.ValueString()
			}
			return origin.ServerName.ValueString()
		}()),
	}

	if _, err := r.client.UpdateDomainConfig(deleteDomainConfigRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CDN Domain",
			err.Error(),
		)
		return
	}

	decribeDomainsConfig := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	decribeDomainsConfig.Filters = []*tencentCloudCdnClient.DomainFilter{
		{
			Name:  common.StringPtr("domain"),
			Value: common.StringPtrs([]string{state.DomainName.ValueString()}),
		},
	}

	checkStatus := func() (bool, error) {
		response, err := r.client.DescribeDomainsConfig(decribeDomainsConfig)
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

	timeout := 15 * time.Minute
	startTime := time.Now()

	for {
		isOnline, err := checkStatus()
		if err != nil {
			resp.Diagnostics.AddError(
				"[API ERROR] Error checking CDN domain status",
				err.Error(),
			)
			return
		}

		if isOnline {
			fmt.Println("Domain status is online.")
			break
		}

		if time.Since(startTime) > timeout {
			resp.Diagnostics.AddError(
				"[TIMEOUT] Timed out waiting for domain status to become online",
				"Reached the maximum timeout of 15 minutes.",
			)
			return
		}

		time.Sleep(30 * time.Second)
	}

	resp.State.RemoveResource(ctx)
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

	state := &cdnPathBasedRuleResourceModel{}
	state.DomainName = plan.DomainName
	state.Origin = plan.Origin

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (d *cdnPathBasedRuleResource) updateDomainConfig(plan *cdnPathBasedRuleResourceModel) error {
	updateDomainConfigRequest := tencentCloudCdnClient.NewUpdateDomainConfigRequest()
	if updateDomainConfigRequest == nil {
		return fmt.Errorf("failed to create a new UpdateDomainConfigRequest")
	}

	if plan.DomainName.ValueString() == "" {
		return fmt.Errorf("domain name cannot be empty")
	}

	for _, origin := range plan.Origin {
		outerOrigins := make([]string, len(origin.Origins.Elements()))
		for i, o := range origin.Origins.Elements() {
			outerOrigins[i] = strings.Trim(o.(types.String).ValueString(), "\"")
		}

		updateDomainConfigRequest.Domain = common.StringPtr(plan.DomainName.ValueString())
		updateDomainConfigRequest.Origin = &tencentCloudCdnClient.Origin{
			Origins:    common.StringPtrs(outerOrigins),
			OriginType: common.StringPtr(origin.OriginType.ValueString()),
			ServerName: common.StringPtr(func() string {
				if origin.ServerName.ValueString() == "" {
					return plan.DomainName.ValueString()
				}
				return origin.ServerName.ValueString()
			}()),
		}

		var pathBasedOriginRules []*tencentCloudCdnClient.PathBasedOriginRule

		for _, pathBasedRule := range origin.PathBasedrule {
			rulePaths := make([]string, len(pathBasedRule.RulePaths.Elements()))
			for i, rp := range pathBasedRule.RulePaths.Elements() {
				rulePaths[i] = strings.Trim(rp.(types.String).ValueString(), "\"")
			}

			innerOrigins := make([]string, len(pathBasedRule.Origin.Elements()))
			for i, o := range pathBasedRule.Origin.Elements() {
				innerOrigins[i] = strings.Trim(o.(types.String).ValueString(), "\"")
			}

			if len(rulePaths) == 0 || len(innerOrigins) == 0 {
				return fmt.Errorf("both rule paths and origins must be provided")
			}

			pathBasedOriginRules = append(pathBasedOriginRules, &tencentCloudCdnClient.PathBasedOriginRule{
				RuleType:  common.StringPtr(pathBasedRule.RuleType.ValueString()),
				RulePaths: common.StringPtrs(rulePaths),
				Origin:    common.StringPtrs(innerOrigins),
			})
		}

		updateDomainConfigRequest.Origin.PathBasedOrigin = pathBasedOriginRules
	}

	updateDomainConfigRequest.ProjectId = common.Int64Ptr(0)

	updateDomainConfig := func() error {
		_, err := d.client.UpdateDomainConfig(updateDomainConfigRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok && isAbleToRetry(t.GetCode()) {
				return err
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(updateDomainConfig, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to update domain config: %w", err)
	}

	decribeDomainsConfig := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	decribeDomainsConfig.Filters = []*tencentCloudCdnClient.DomainFilter{
		{
			Name:  common.StringPtr("domain"),
			Value: common.StringPtrs([]string{plan.DomainName.ValueString()}),
		},
	}

	checkStatus := func() (bool, error) {
		response, err := d.client.DescribeDomainsConfig(decribeDomainsConfig)
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

	timeout := 15 * time.Minute
	startTime := time.Now()

	for {
		isOnline, err := checkStatus()
		if err != nil {
			return fmt.Errorf("error checking CDN domain status: %w", err)
		}

		if isOnline {
			fmt.Println("Domain is now online.")
			break
		}

		if time.Since(startTime) > timeout {
			return fmt.Errorf("timed out waiting for domain status to become online")
		}

		time.Sleep(30 * time.Second)
	}

	return nil
}
