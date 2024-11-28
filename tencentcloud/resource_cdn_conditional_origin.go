package tencentcloud

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	_     resource.Resource               = &cdnConditionalOriginResource{}
	_     resource.ResourceWithConfigure  = &cdnConditionalOriginResource{}
	_     resource.ResourceWithModifyPlan = &cdnConditionalOriginResource{}
	mutex sync.Mutex
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

/*
The use of blocks helps to reduce waiting time. If a separate resource is created for each rule, it would significantly
increase the overall time required, as each resource must wait until the domain name's status becomes "online" before it
can be created. Otherwise, an error would occur, indicating that the domain being configured cannot be modified.
*/
func (r *cdnConditionalOriginResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a TencentCloud conditional origin resource.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Domain name.",
				Required:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"rule": schema.SetNestedBlock{
				Description: "Configuration of rule.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"origin": schema.StringAttribute{
							Description: "Address of the origin server from which the CDN retrieves content when it is not cached.",
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
							Description: "Rule path that specifies conditions under which the CDN retrieves content from a designated origin address.",
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
			"[API ERROR] Failed to Create Conditional Origin.",
			err.Error(),
		)
		return
	}

	setStateDiags := resp.State.Set(ctx, &plan)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	/*
		This update is intended to remove the resource; therefore, it is necessary to wait until the domain is online.
		Otherwise, an error will occur during terraform destroy.
	*/
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
	reconnectBackoff.MaxElapsedTime = 30 * time.Minute
	err := backoff.Retry(describeDomainsConfig, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Conditional Origin.",
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
			"[API ERROR] Failed to update Conditional Origin.",
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

	deleteConditionalOriginRequest, err := buildConditionalOriginRequest(state, originTest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[ERROR] Failed to build CDN confitional origin.",
			err.Error(),
		)
		return
	}

	/* The reason for using UpdateDomainConfig is that Tencent Cloud only provides UpdateDomainConfig to remove
	path-based origin rules or path rules; they only have a separate DeleteScdnDomain function exclusively for deleting an entire
	domain name, not only remove path based origin rule or path rules.*/
	deleteConditionalOriginRequest.Origin.PathBasedOrigin = nil
	deleteConditionalOriginRequest.Origin.PathRules = nil
	if _, err := r.client.UpdateDomainConfig(deleteConditionalOriginRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to delete Conditional Origin.",
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

func (r *cdnConditionalOriginResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		fmt.Println("Plan is null; skipping ModifyPlan.")
		return
	}

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

	for i, rule := range plan.Rule {
		ruleType := rule.RuleType.ValueString()
		rulePath := rule.RulePath.ValueString()

		if ruleType == "file" {
			if strings.HasPrefix(rulePath, "/") || strings.HasPrefix(rulePath, ".") {
				errMsg := fmt.Sprintf(
					"Validation Error in rule[%d]: When 'rule_type' is 'file', 'rule_path' cannot start with '/' or '.'. Got: %s",
					i, rulePath,
				)
				resp.Diagnostics.AddError("Validation Error", errMsg)
				fmt.Println(errMsg)
				return
			}
		} else if ruleType == "path" || ruleType == "directory" {
			if !strings.HasPrefix(rulePath, "/") {
				errMsg := fmt.Sprintf(
					"Validation Error in rule[%d]: When 'rule_type' is '%s', 'rule_path' must start with '/'. Got: %s",
					i, ruleType, rulePath,
				)
				resp.Diagnostics.AddError("Validation Error", errMsg)
				fmt.Println(errMsg)
				return
			}
			// Validate 'rule_path' does not end with '/'
			if strings.HasSuffix(rulePath, "/") {
				errMsg := fmt.Sprintf(
					"Validation Error in rule[%d]: When 'rule_type' is '%s', 'rule_path' cannot end with '/'. Got: %s",
					i, ruleType, rulePath,
				)
				resp.Diagnostics.AddError("Validation Error", errMsg)
				fmt.Println(errMsg)
				return
			}
		} else if ruleType == "index" {
			if rulePath != "/" {
				errMsg := fmt.Sprintf(
					"Validation Error in rule[%d]: When 'rule_type' is 'index', 'rule_path' must be exactly '/'. Got: %s",
					i, rulePath,
				)
				resp.Diagnostics.AddError("Validation Error", errMsg)
				fmt.Println(errMsg)
				return
			}
		} else {
			fmt.Printf("Skipping validation for rule[%d]: Unsupported rule_type '%s'.\n", i, ruleType)
		}
	}

}

func (d *cdnConditionalOriginResource) updateDomainConfig(plan *cdnConditionalOriginResourceModel) error {
	resp := &resource.DeleteResponse{}
	originTest, fetchErr := fetchAndMapDomainConfig(d, plan, resp)
	if fetchErr != nil {
		return fmt.Errorf("failed to fetch and map domain configuration: %w", fetchErr)
	}

	updateDomainConfigRequest, err := buildConditionalOriginRequest(plan, originTest)
	if err != nil {
		return fmt.Errorf("failed to build conditional origin: %w", err)
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
	reconnectBackoff.MaxElapsedTime = 30 * time.Minute
	err = backoff.Retry(updateDomainConfig, reconnectBackoff)
	if err != nil {
		return fmt.Errorf("failed to update conditional origin: %w", err)
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
	var pathRule []*tencentCloudCdnClient.PathRule

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

			pathRule = append(pathRule, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(formattedPath),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(formattedForwardUri),
				Regex:      common.BoolPtr(true),
				FullMatch:  common.BoolPtr(false),
			})
		} else if rule.RuleType.ValueString() == "directory" {
			formattedPath := rule.RulePath.ValueString() + "/*"
			formattedForwardUri := rule.RulePath.ValueString() + "/$1"

			pathRule = append(pathRule, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(formattedPath),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(formattedForwardUri),
				Regex:      common.BoolPtr(true),
				FullMatch:  common.BoolPtr(false),
			})
		} else if rule.RuleType.ValueString() == "path" {
			pathRule = append(pathRule, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr(rule.RulePath.ValueString()),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr(rule.RulePath.ValueString()),
				Regex:      common.BoolPtr(false),
				FullMatch:  common.BoolPtr(true),
			})
		} else if rule.RuleType.ValueString() == "index" {
			pathRule = append(pathRule, &tencentCloudCdnClient.PathRule{
				Path:       common.StringPtr("/"),
				ServerName: common.StringPtr(rule.Origin.ValueString()),
				ForwardUri: common.StringPtr("/"),
				Regex:      common.BoolPtr(true),
				FullMatch:  common.BoolPtr(false),
			})
		}
	}

	updateConditionalOriginRequest.Origin = &tencentCloudCdnClient.Origin{
		Origins:            common.StringPtrs([]string{originTest[0].OriginList.ValueString()}),
		OriginType:         common.StringPtr(originTest[0].OriginType.ValueString()),
		OriginPullProtocol: common.StringPtr(originTest[0].OriginPullProtocol.ValueString()),
		ServerName:         common.StringPtr(plan.DomainName.ValueString()),
		PathBasedOrigin:    pathBasedOriginRules,
		PathRules:          pathRule,
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
		mutex.Lock()
		defer mutex.Unlock()

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
	reconnectBackoff.MaxElapsedTime = 30 * time.Minute

	err := backoff.Retry(describeCdnDomain, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Conditional Origin.",
			err.Error(),
		)
		return nil, err
	}

	var result []originStruct
	for _, domain := range response.Response.Domains {
		origin := originStruct{
			OriginList:         types.StringValue(strings.Join(common.StringValues(domain.Origin.Origins), ",")),
			OriginType:         types.StringValue(*domain.Origin.OriginType),
			ServerName:         types.StringValue(*domain.Origin.ServerName),
			OriginPullProtocol: types.StringValue(*domain.Origin.OriginPullProtocol),
		}
		result = append(result, origin)
	}

	return result, nil
}
