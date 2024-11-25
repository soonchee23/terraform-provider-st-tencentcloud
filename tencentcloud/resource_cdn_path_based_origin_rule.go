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
	_ resource.Resource               = &cdnConditionalOriginResource{}
	_ resource.ResourceWithConfigure  = &cdnConditionalOriginResource{}
	_ resource.ResourceWithModifyPlan = &cdnConditionalOriginResource{}
)

func NewCdnConditionalOriginResource() resource.Resource {
	return &cdnConditionalOriginResource{}
}

type cdnConditionalOriginResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnConditionalOriginResourceModel struct {
	DomainName types.String `tfsdk:"domain"`
	Rule       []*rule      `tfsdk:"rule"`
}

type rule struct {
	Origin   types.String `tfsdk:"origin"`
	RuleType types.String `tfsdk:"rule_type"`
	RulePath types.String `tfsdk:"rule_path"`
}

type originStruct struct {
	OriginList         types.String `json:"origin_list"`
	OriginType         types.String `json:"origin_type"`
	ServerName         types.String `json:"server_name"`
	OriginPullProtocol types.String `json:"origin_pull_protocol"`
}

func (r *cdnConditionalOriginResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_conditional_origin"
}

func (r *cdnConditionalOriginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a TencentCloud Path-Based Rule resource.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"rule": schema.SetNestedBlock{
				Description: "Configuration of rules.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"origin": schema.StringAttribute{
							Description: "Origin.",
							Required:    true,
						},
						"rule_type": schema.StringAttribute{
							Description: "Type of the rule for conditional origin.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("file", "directory", "path", "index"),
							},
						},
						"rule_path": schema.StringAttribute{
							Description: "Rule path.",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

func (r *cdnConditionalOriginResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (r *cdnConditionalOriginResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cdnConditionalOriginResourceModel
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

	/*This update is intended to remove the resource; therefore, it is necessary to wait until the domain is online. Otherwise, an error will occur during terraform destroy.*/
	err = waitForCDNDomainStatus(r.client, plan.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"[TIMEOUT] Timed out waiting for domain status to become online",
			err.Error(),
		)
		return
	}
}

func (r *cdnConditionalOriginResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *cdnConditionalOriginResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeDomainsConfigRequest := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	if !(state.DomainName.IsUnknown() || state.DomainName.IsNull()) {
		describeDomainsConfigRequest.Filters = []*tencentCloudCdnClient.DomainFilter{makeDomainFilter("domain", state.DomainName.ValueString())}
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

func (r *cdnConditionalOriginResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *cdnConditionalOriginResourceModel
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

func (r *cdnConditionalOriginResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *cdnConditionalOriginResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	originTest, fetchErr := fetchAndMapDomainConfig(r, state, resp)
	if fetchErr != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to fetch and map domain configuration",
			fetchErr.Error(),
		)
		return
	}

	// Debug log to check if originTest is empty
	fmt.Printf("[DEBUG] originTest: %+v\n", originTest)

	// Build delete request ignoring originTest if it's empty
	deleteConditionalOriginRequest, err := buildConditionalOriginRequest(state, originTest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to build CDN path-based origin rule",
			err.Error(),
		)
		return
	}

	// Set PathBasedOrigin and PathRules to nil regardless of originTest
	deleteConditionalOriginRequest.Origin.PathBasedOrigin = nil
	deleteConditionalOriginRequest.Origin.PathRules = nil

	// Call API to update domain config
	if _, err := r.client.UpdateDomainConfig(deleteConditionalOriginRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to delete CDN domain",
			err.Error(),
		)
		return
	}

	// Wait for the domain status to become online
	err = waitForCDNDomainStatus(r.client, state.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		resp.Diagnostics.AddError(
			"[TIMEOUT] Timed out waiting for domain status to become online",
			err.Error(),
		)
		return
	}
}

func (r *cdnConditionalOriginResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if req.Plan.Raw.IsNull() {
		fmt.Println("Plan is null; skipping ModifyPlan.")
		return
	}

	// Retrieve the planned state into a cdnConditionalOriginResourceModel structure
	var plan cdnConditionalOriginResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		fmt.Println("Error retrieving the plan.")
		return
	}

	if len(plan.Rule) == 0 {
		errMsg := "Validation Error: At least one 'rule' block must be specified."
		resp.Diagnostics.AddError("Validation Error", errMsg)
		fmt.Println(errMsg)
		return
	}

}

func (d *cdnConditionalOriginResource) updateDomainConfig(plan *cdnConditionalOriginResourceModel) error {
	resp := &resource.DeleteResponse{} // Create a placeholder DeleteResponse object
	originTest, fetchErr := fetchAndMapDomainConfig(d, plan, resp)
	if fetchErr != nil {
		return fmt.Errorf("failed to fetch and map domain configuration: %w", fetchErr)
	}

	// Use the fetched `originList` as needed (e.g., logging or validation)
	fmt.Printf("Fetched origins: %+v\n", originTest)

	updateDomainConfigRequest, err := buildConditionalOriginRequest(plan, originTest)
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
		return fmt.Errorf("failed to update path based origin rule: %w", err)
	}

	err = waitForCDNDomainStatus(d.client, plan.DomainName.ValueString(), 15*time.Minute)
	if err != nil {
		return err
	}

	return nil
}

func buildConditionalOriginRequest(plan *cdnConditionalOriginResourceModel, originTest []originStruct) (*tencentCloudCdnClient.UpdateDomainConfigRequest, error) {
	if plan.DomainName.ValueString() == "" {
		return nil, fmt.Errorf("domain name cannot be empty")
	}

	updateConditionalOriginRequest := tencentCloudCdnClient.NewUpdateDomainConfigRequest()
	updateConditionalOriginRequest.Domain = common.StringPtr(plan.DomainName.ValueString())
	updateConditionalOriginRequest.ProjectId = common.Int64Ptr(0)

	var pathBasedOriginRules []*tencentCloudCdnClient.PathBasedOriginRule
	var rewritePathRules []*tencentCloudCdnClient.PathRule

	for _, rule := range plan.Rule {
		ruleList := strings.Split(strings.Trim(rule.RulePath.ValueString(), "\""), ",")
		for i := range ruleList {
			ruleList[i] = strings.TrimSpace(ruleList[i])
		}

		originList := strings.Split(strings.Trim(rule.Origin.ValueString(), "\""), ",")
		for i := range originList {
			originList[i] = strings.TrimSpace(originList[i])
		}

		pathBasedOriginRules = append(pathBasedOriginRules, &tencentCloudCdnClient.PathBasedOriginRule{
			RuleType:  common.StringPtr(rule.RuleType.ValueString()),
			RulePaths: common.StringPtrs(ruleList),
			Origin:    common.StringPtrs(originList),
		})

		if rule.RuleType.ValueString() == "file" {
			formattedPath := "/*." + rule.RulePath.ValueString()
			formattedForwardUri := "/$1." + rule.RulePath.ValueString()

			rewritePathRules = append(rewritePathRules, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(formattedPath),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(formattedForwardUri),
				Regex:      common.BoolPtr(true),
				FullMatch:  common.BoolPtr(false),
			})

		} else if rule.RuleType.ValueString() == "directory" {
			formattedPath := rule.RulePath.ValueString() + "/*"
			formattedForwardUri := rule.RulePath.ValueString() + "/$1"

			rewritePathRules = append(rewritePathRules, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(formattedPath),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(formattedForwardUri),
				Regex:      common.BoolPtr(true),
				FullMatch:  common.BoolPtr(false),
			})

		} else if rule.RuleType.ValueString() == "path" {
			rewritePathRules = append(rewritePathRules, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(rule.RulePath.ValueString()),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(rule.RulePath.ValueString()),
				Regex:      common.BoolPtr(false),
				FullMatch:  common.BoolPtr(true),
			})
		}
	}

	updateConditionalOriginRequest.Origin = &tencentCloudCdnClient.Origin{
		Origins:            common.StringPtrs([]string{originTest[0].OriginList.ValueString()}),
		OriginType:         common.StringPtr(originTest[0].OriginType.ValueString()),
		OriginPullProtocol: common.StringPtr(originTest[0].OriginPullProtocol.ValueString()),
		ServerName:         common.StringPtr(plan.DomainName.ValueString()),
		PathBasedOrigin:    pathBasedOriginRules,
		PathRules:          rewritePathRules,
	}

	return updateConditionalOriginRequest, nil
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

func fetchAndMapDomainConfig(d *cdnConditionalOriginResource, plan *cdnConditionalOriginResourceModel, resp *resource.DeleteResponse) ([]originStruct, error) {
	request := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	if plan.DomainName.ValueString() != "" {
		request.Filters = append(request.Filters, makeDomainFilter("domain", plan.DomainName.ValueString()))
	}

	response := tencentCloudCdnClient.NewDescribeDomainsConfigResponse()
	describeCdnDomain := func() error {
		var err error
		response, err = d.client.DescribeDomainsConfig(request)
		if err != nil {
			if terr, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(terr.GetCode()) {
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

	err := backoff.Retry(describeCdnDomain, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Load Balancers",
			err.Error(),
		)
		return nil, err
	}

	var result []originStruct
	for _, domain := range response.Response.Domains {
		origin := originStruct{
			OriginList:         types.StringValue(convertOriginsToString(domain.Origin.Origins)),
			OriginType:         types.StringValue(*domain.Origin.OriginType),
			ServerName:         types.StringValue(*domain.Origin.ServerName),
			OriginPullProtocol: types.StringValue(*domain.Origin.OriginPullProtocol),
		}
		result = append(result, origin)
	}

	return result, nil
}

// Helper function to convert []*string to a single string
func convertOriginsToString(origins []*string) string {
	var result []string
	for _, origin := range origins {
		if origin != nil {
			result = append(result, *origin)
		}
	}
	return strings.Join(result, ",") // Join with a comma or any delimiter you prefer
}
