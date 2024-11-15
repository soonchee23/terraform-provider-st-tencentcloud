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
	_ resource.Resource               = &cdnPathBasedOriginRuleResource{}
	_ resource.ResourceWithConfigure  = &cdnPathBasedOriginRuleResource{}
	_ resource.ResourceWithModifyPlan = &cdnPathBasedOriginRuleResource{}
)

func NewCdnPathBasedOriginRuleResource() resource.Resource {
	return &cdnPathBasedOriginRuleResource{}
}

type cdnPathBasedOriginRuleResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnPathBasedOriginRuleResourceModel struct {
	DomainName types.String `tfsdk:"domain"`
	Origin     []*origin    `tfsdk:"origin"`
}

type origin struct {
	Origins             types.List             `tfsdk:"origin_list"`
	OriginType          types.String           `tfsdk:"origin_type"`
	ServerName          types.String           `tfsdk:"server_name"`
	OriginPullProtocol  types.String           `tfsdk:"origin_pull_protocol"`
	PathBasedOriginRule []*pathBasedOriginRule `tfsdk:"path_based_origin_rule"`
	RewritePathRule     []*rewritePathRule     `tfsdk:"rewrite_path_rule"`
}

type pathBasedOriginRule struct {
	RuleType  types.String `tfsdk:"rule_type"`
	RulePaths types.List   `tfsdk:"rule_paths"`
	Origin    types.String `tfsdk:"origin"`
}

type rewritePathRule struct {
	Regex          types.Bool        `tfsdk:"regex"`
	Path           types.String      `tfsdk:"path"`
	Origin         types.String      `tfsdk:"origin"`
	ServerName     types.String      `tfsdk:"server_name"`
	OriginArea     types.String      `tfsdk:"origin_area"`
	ForwardUri     types.String      `tfsdk:"forward_uri"`
	FullMatch      types.Bool        `tfsdk:"full_match"`
	RequestHeaders []*httpHeaderRule `tfsdk:"request_headers"`
}

type httpHeaderRule struct {
	HeaderMode  types.String `tfsdk:"header_mode"`
	HeaderName  types.String `tfsdk:"header_name"`
	HeaderValue types.String `tfsdk:"header_value"`
}

func (r *cdnPathBasedOriginRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_path_based_origin_rule"
}

func (r *cdnPathBasedOriginRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
				Description: "Origin Configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"origin_list": schema.ListAttribute{
							ElementType: types.StringType,
							Description: "Origin list for the CDN domain.",
							Required:    true,
						},
						"origin_type": schema.StringAttribute{
							Description: "Type of origin.",
							Required:    true,
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
						"origin_pull_protocol": schema.StringAttribute{
							Description: "Origin pull protocol.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									"http",
									"https",
									"follow"),
							},
						},
					},
					Blocks: map[string]schema.Block{
						"path_based_origin_rule": schema.ListNestedBlock{
							Description: "Path based origin rule configuration.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"origin": schema.StringAttribute{
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
						"rewrite_path_rule": schema.ListNestedBlock{
							Description: "Rewrite path rule configuration.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"regex": schema.BoolAttribute{
										Description: "A pattern used to search, replace, or validate parts of a string.",
										Required:    true,
									},
									"path": schema.StringAttribute{
										Description: "Matched URL paths only support URL paths and do not support parameters. The default is exact matching; when wildcard “*” matching is enabled, it supports up to 5 wildcards with a maximum length of 1024 characters.",
										Required:    true,
									},
									"origin": schema.StringAttribute{
										Description: "The origin site for path matching does not currently support COS sources with private read/write access. If not specified, the default origin site will be used.",
										Optional:    true,
									},
									"server_name": schema.StringAttribute{
										Description: "The Host header for the origin during path matching. If not specified, the default ServerName will be used.",
										Required:    true,
									},
									"origin_area": schema.StringAttribute{
										Description: "The region of the origin site, supporting CN and OV.",
										Required:    true,
									},
									"forward_uri": schema.StringAttribute{
										Description: "The URI path for the origin during path matching must start with “/” and cannot include parameters.",
										Required:    true,
									},
									"full_match": schema.BoolAttribute{
										Description: "Ensures the entire string exactly matches a given pattern, with no extra characters.",
										Required:    true,
									},
								},
								Blocks: map[string]schema.Block{
									"request_headers": schema.SetNestedBlock{
										Description: "The alert notification methods. See the following Block alert_config.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"header_mode": schema.StringAttribute{
													Description: "The HTTP header configuration supports three methods: add, set, and del, which respectively mean adding a new header, setting (or modifying) an existing header, and deleting a header.",
													Optional:    true,
												},
												"header_name": schema.StringAttribute{
													Description: "HTTP header name.",
													Optional:    true,
												},
												"header_value": schema.StringAttribute{
													Description: "HTTP header value.",
													Optional:    true,
												},
											},
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

func (r *cdnPathBasedOriginRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (r *cdnPathBasedOriginRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cdnPathBasedOriginRuleResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.updateDomainConfig(&plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to update path based origin rule",
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

func (r *cdnPathBasedOriginRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *cdnPathBasedOriginRuleResourceModel
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
	reconnectBackoff.MaxElapsedTime = 5 * time.Minute
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

func (r *cdnPathBasedOriginRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *cdnPathBasedOriginRuleResourceModel
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

func (r *cdnPathBasedOriginRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *cdnPathBasedOriginRuleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	/*The reason for using UpdateDomainConfig is that Tencent Cloud only provides this API to remove
	path-based rules; they have a separate DeleteScdnDomain function exclusively for deleting an entire
	domain name.*/
	deletePathBasedOriginRuleRequest, err := buildPathBasedOriginRuleRequest(state)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to build CDN path based origin rule",
			err.Error(),
		)
		return
	}

	deletePathBasedOriginRuleRequest.Origin.PathBasedOrigin = nil
	if _, err := r.client.UpdateDomainConfig(deletePathBasedOriginRuleRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CDN Domain",
			err.Error(),
		)
		return
	}

	/*This update is intended to remove the resource; therefore, it is necessary to wait until the domain is online. Otherwise, an error will occur during terraform destroy.*/
	err = waitForCDNDomainStatus(r.client, state.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"[TIMEOUT] Timed out waiting for domain status to become online",
			err.Error(),
		)
		return
	}
}

func (r *cdnPathBasedOriginRuleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if req.Plan.Raw.IsNull() {
		fmt.Println("Plan is null; skipping ModifyPlan.")
		return
	}

	// Retrieve the planned state into a cdnPathBasedOriginRuleResourceModel structure
	var plan cdnPathBasedOriginRuleResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		fmt.Println("Error retrieving the plan.")
		return
	}

	for originIndex, origin := range plan.Origin {
		fmt.Printf("Origin %d: %+v\n", originIndex+1, origin)
		for urlIndex, urlConfig := range origin.RewritePathRule {
			fmt.Printf("Checking rewrite_path_rule %d in origin %d - FullMatch: %v, Regex: %v\n",
				urlIndex+1, originIndex+1, urlConfig.FullMatch.ValueBool(), urlConfig.Regex.ValueBool())

			// Check if FullMatch and Regex are the same (either both true or both false)
			if urlConfig.FullMatch.ValueBool() == urlConfig.Regex.ValueBool() {
				errMsg := fmt.Sprintf(
					"Validation Error in origin %d, rewrite_path_rule %d: either FullMatch or Regex must be true, but not both; please ensure that one of them is true and the other is false.",
					originIndex+1, urlIndex+1,
				)
				resp.Diagnostics.AddError("Validation Error", errMsg)
				fmt.Println(errMsg)
				return
			}
		}
	}
}

func (d *cdnPathBasedOriginRuleResource) updateDomainConfig(plan *cdnPathBasedOriginRuleResourceModel) error {
	updateDomainConfigRequest, err := buildPathBasedOriginRuleRequest(plan)
	if err != nil {
		return fmt.Errorf("failed to build path based origin rule: %w", err)
	}

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
	reconnectBackoff.MaxElapsedTime = 5 * time.Minute
	err = backoff.Retry(updateDomainConfig, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to update path based origin rulee: %w", err)
	}

	err = waitForCDNDomainStatus(d.client, plan.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		return err
	}

	return nil
}

func buildPathBasedOriginRuleRequest(plan *cdnPathBasedOriginRuleResourceModel) (*tencentCloudCdnClient.UpdateDomainConfigRequest, error) {
	if plan.DomainName.ValueString() == "" {
		return nil, fmt.Errorf("domain name cannot be empty")
	}

	updatePathBasedOriginRuleRequest := tencentCloudCdnClient.NewUpdateDomainConfigRequest()
	updatePathBasedOriginRuleRequest.Domain = common.StringPtr(plan.DomainName.ValueString())

	for _, origin := range plan.Origin {
		mainOrigins := make([]string, len(origin.Origins.Elements()))
		for i, o := range origin.Origins.Elements() {
			mainOrigins[i] = strings.Trim(o.(types.String).ValueString(), "\"")
		}

		var pathBasedOriginRules []*tencentCloudCdnClient.PathBasedOriginRule
		for _, pathBasedOriginRule := range origin.PathBasedOriginRule {
			rulePaths := make([]string, len(pathBasedOriginRule.RulePaths.Elements()))
			for i, rp := range pathBasedOriginRule.RulePaths.Elements() {
				rulePaths[i] = strings.Trim(rp.(types.String).ValueString(), "\"")
			}

			pathOriginStr := pathBasedOriginRule.Origin.ValueString()
			pathOrigins := strings.Split(strings.Trim(pathOriginStr, "\""), ",")

			for i := range pathOrigins {
				pathOrigins[i] = strings.TrimSpace(pathOrigins[i])
			}

			if len(rulePaths) == 0 || len(pathOrigins) == 0 {
				return nil, fmt.Errorf("both rule paths and origins must be provided")
			}

			pathBasedOriginRules = append(pathBasedOriginRules, &tencentCloudCdnClient.PathBasedOriginRule{
				RuleType:  common.StringPtr(pathBasedOriginRule.RuleType.ValueString()),
				RulePaths: common.StringPtrs(rulePaths),
				Origin:    common.StringPtrs(pathOrigins),
			})
		}

		var rewritePathRule []*tencentCloudCdnClient.PathRule
		for _, pathRule := range origin.RewritePathRule {

			/*full match & regex 只可以是相反的，不能同时是false或则true
			原因如下：
			1. Full Path Matching is meant to match a single, specific URL exactly as written.
				Full Path Matching Rule: /products
					- Matches:
						- /products

					- Does not match:
						- /products?category=electronics (because it includes a query string)

			2. Regex Matching allows for patterns and variability, matching multiple potential URLs that fit a defined pattern.
				Regex Matching Rule: ^/products(\?.*)?$
					- Matches:
						- /products
						- /products?category=electronics
					- Does not match:
						- /product-list
			*/

			if pathRule.FullMatch.ValueBool() == pathRule.Regex.ValueBool() {
				return nil, fmt.Errorf("either FullMatch or Regex must be true, but not both; please ensure that one of them is true and the other is false")
			}

			var originValue *string
			if pathRule.Origin.ValueString() == "" {
				originValue = nil
			} else {
				originValue = common.StringPtr(pathRule.Origin.ValueString())
			}

			var requestHeaders []*tencentCloudCdnClient.HttpHeaderRule
			for _, header := range pathRule.RequestHeaders {
				if header.HeaderMode.ValueString() == "" && header.HeaderName.ValueString() == "" && header.HeaderValue.ValueString() == "" {
					continue
				}
				requestHeaders = append(requestHeaders, &tencentCloudCdnClient.HttpHeaderRule{
					HeaderMode:  common.StringPtr(header.HeaderMode.ValueString()),
					HeaderName:  common.StringPtr(header.HeaderName.ValueString()),
					HeaderValue: common.StringPtr(header.HeaderValue.ValueString()),
				})
			}

			rewritePathRule = append(rewritePathRule, &tencentCloudCdnClient.PathRule{
				Path:           common.StringPtr(pathRule.Path.ValueString()),
				Origin:         originValue,
				ServerName:     common.StringPtr(pathRule.ServerName.ValueString()),
				OriginArea:     common.StringPtr(pathRule.OriginArea.ValueString()),
				ForwardUri:     common.StringPtr(pathRule.ForwardUri.ValueString()),
				RequestHeaders: requestHeaders,
				Regex:          common.BoolPtr(pathRule.Regex.ValueBool()),
				FullMatch:      common.BoolPtr(pathRule.FullMatch.ValueBool()),
			})
		}

		/*The Origin List, Origin Type, and Server Name need to be specified again, as they are part of the
		tencentcloud_cdn_domains resource in the Tencent Cloud Terraform provider. This is necessary because
		the path-based rule is nested within the origin configuration, and it's important to associate the
		path-based rule with the correct origin.*/
		updatePathBasedOriginRuleRequest.Origin = &tencentCloudCdnClient.Origin{
			Origins:            common.StringPtrs(mainOrigins),
			OriginType:         common.StringPtr(origin.OriginType.ValueString()),
			OriginPullProtocol: common.StringPtr(origin.OriginPullProtocol.ValueString()),
			ServerName: common.StringPtr(func() string {
				if origin.ServerName.ValueString() == "" {
					return plan.DomainName.ValueString()
				}
				return origin.ServerName.ValueString()
			}()),
			PathBasedOrigin: pathBasedOriginRules,
			PathRules:       rewritePathRule,
		}
	}

	updatePathBasedOriginRuleRequest.ProjectId = common.Int64Ptr(0)
	return updatePathBasedOriginRuleRequest, nil
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
