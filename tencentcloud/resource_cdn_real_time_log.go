package tencentcloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tencentCloudCdnClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

var (
	_ resource.Resource                = &cdnRealTimeLogResource{}
	_ resource.ResourceWithConfigure   = &cdnRealTimeLogResource{}
	_ resource.ResourceWithModifyPlan  = &cdnRealTimeLogResource{}
	_ resource.ResourceWithImportState = &cdnRealTimeLogResource{}
)

func NewCdnRealTimeLogResource() resource.Resource {
	return &cdnRealTimeLogResource{}
}

type cdnRealTimeLogResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnRealTimeLogResourceModel struct {
	TopicName types.String `tfsdk:"topic_name"`
	Area      types.String `tfsdk:"area"`
	TopicId   types.String `tfsdk:"topic_id"`
	LogsetId  types.String `tfsdk:"logset_id"`
	Domains   types.List   `tfsdk:"domains"`
}

func (r *cdnRealTimeLogResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_real_time_log"
}

func (r *cdnRealTimeLogResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tencent Cloud CDN Real Time Log resource.",
		Attributes: map[string]schema.Attribute{
			"topic_name": schema.StringAttribute{
				Description: "The name of the CDN Real Time Log to be created.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"area": schema.StringAttribute{
				Description: "The area associated with the domain (only 'mainland' or 'overseas' allowed).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("mainland", "overseas"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domains": schema.ListAttribute{
				Description: "The domain name associated with this log configuration.",
				Required:    true,
				ElementType: types.StringType,
			},
			"topic_id": schema.StringAttribute{
				Description: "The unique identifier of the CDN Real Time Log, assigned by Tencent Cloud.",
				Computed:    true,
			},
			"logset_id": schema.StringAttribute{
				Description: "The ID of the CDN Real Time Log logset where the log topic is stored.",
				Computed:    true,
			},
		},
	}
}

func (r *cdnRealTimeLogResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (r *cdnRealTimeLogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan cdnRealTimeLogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainSet := make(map[string]struct{})
	duplicateDomains := make(map[string]int)
	for _, domain := range plan.Domains.Elements() {
		domainStr := domain.(types.String).ValueString()

		if _, exists := domainSet[domainStr]; exists {
			duplicateDomains[domainStr]++
		} else {
			domainSet[domainStr] = struct{}{}
			duplicateDomains[domainStr] = 1
		}
	}

	var duplicates []string
	for domain, count := range duplicateDomains {
		if count > 1 {
			duplicates = append(duplicates, domain)
		}
	}

	if len(duplicates) > 0 {
		resp.Diagnostics.AddError(
			"Duplicate Domains Found",
			fmt.Sprintf("The following domains are defined multiple times in the domains list: %v", duplicates),
		)
	}
}

func (r *cdnRealTimeLogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state cdnRealTimeLogResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createClsLogTopicResponse, err := r.createClsLogTopic(&plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Create CLS Log Topic.",
			err.Error(),
		)
		return
	}

	if createClsLogTopicResponse.Response.TopicId == nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Missing TopicId in response.",
			"Received a response from the API, but TopicId is nil.",
		)
		return
	}

	state = plan
	state.TopicId = types.StringValue(*createClsLogTopicResponse.Response.TopicId)
	state.LogsetId = types.StringValue(getLogsetId(plan.Area.ValueString()))
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnRealTimeLogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cdnRealTimeLogResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("[ERROR] Unable to retrieve state", "State is nil or invalid.")
		return
	}

	listClsTopicDomainsRequest := tencentCloudCdnClient.NewListClsTopicDomainsRequest()
	listClsTopicDomainsRequest.LogsetId = common.StringPtr(state.LogsetId.ValueString())
	listClsTopicDomainsRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	listClsTopicDomainsRequest.Channel = common.StringPtr("cdn")

	respData, err := r.client.ListClsTopicDomainsWithContext(ctx, listClsTopicDomainsRequest)
	if err != nil {
		resp.Diagnostics.AddError("[API ERROR] Failed to Describe CDN CLS Topic Domains.", err.Error())
		return
	}

	var domains []attr.Value
	for _, domainConfig := range respData.Response.DomainAreaConfigs {
		if domainConfig.Domain != nil {
			domains = append(domains, types.StringValue(*domainConfig.Domain))
		}
	}

	domainsList, diags := types.ListValue(types.StringType, domains)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.AddError("[ERROR] Failed to convert domains to list", fmt.Sprintf("%v", diags))
		return
	}
	state.Domains = domainsList

	var listClsTopicDomainsResponse *tencentCloudCdnClient.ListClsTopicDomainsResponse
	listClsTopicDomains := func() error {
		var err error
		listClsTopicDomainsResponse, err = r.client.ListClsTopicDomainsWithContext(ctx, listClsTopicDomainsRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		if listClsTopicDomainsResponse == nil || listClsTopicDomainsResponse.Response == nil {
			return fmt.Errorf("empty response from Tencent Cloud")
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(listClsTopicDomains, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError("Failed to retrieve CLS log topic details", err.Error())
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnRealTimeLogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cdnRealTimeLogResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.updateClsLogTopic(&state, &plan); err != nil {
		resp.Diagnostics.AddError("[API ERROR] Failed to Update CDN CLS Topic Domains.", err.Error())
		return
	}

	state.Domains = plan.Domains
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnRealTimeLogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cdnRealTimeLogResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteClsLogTopicRequest := tencentCloudCdnClient.NewDeleteClsLogTopicRequest()
	deleteClsLogTopicRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	deleteClsLogTopicRequest.LogsetId = common.StringPtr(state.LogsetId.ValueString())

	deleteClsLogTopic := func() error {
		if _, err := r.client.DeleteClsLogTopic(deleteClsLogTopicRequest); err != nil {
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

	retryBackoff := backoff.NewExponentialBackOff()
	retryBackoff.MaxElapsedTime = 30 * time.Second

	if err := backoff.Retry(deleteClsLogTopic, retryBackoff); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CLS Log Topic after retries.",
			err.Error(),
		)
	}
}

func (r *cdnRealTimeLogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var area, topicId, logsetId string
	var err error

	area, topicId, err = parseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	logsetId = getLogsetId(area)
	listClsTopicDomainsRequest := tencentCloudCdnClient.NewListClsTopicDomainsRequest()
	listClsTopicDomainsRequest.LogsetId = common.StringPtr(logsetId)
	listClsTopicDomainsRequest.TopicId = common.StringPtr(topicId)
	listClsTopicDomainsRequest.Channel = common.StringPtr("cdn")

	var listClsTopicDomainsResponse *tencentCloudCdnClient.ListClsTopicDomainsResponse
	var topicName string
	var domainList types.List

	listClsTopicDomains := func() error {
		var err error
		listClsTopicDomainsResponse, err = r.client.ListClsTopicDomainsWithContext(ctx, listClsTopicDomainsRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}

		if listClsTopicDomainsResponse == nil || listClsTopicDomainsResponse.Response == nil {
			return fmt.Errorf("empty response from Tencent Cloud")
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(listClsTopicDomains, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError("Failed to retrieve CLS log topic details", err.Error())
		return
	}

	topicName = *listClsTopicDomainsResponse.Response.TopicName

	if len(listClsTopicDomainsResponse.Response.DomainAreaConfigs) > 0 {
		var domains []attr.Value
		for _, domainConfig := range listClsTopicDomainsResponse.Response.DomainAreaConfigs {
			if domainConfig.Domain != nil {
				domains = append(domains, types.StringValue(*domainConfig.Domain))
			}
		}

		var diags diag.Diagnostics
		domainList, diags = types.ListValue(types.StringType, domains)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	} else {
		domainList = types.ListNull(types.StringType)
	}

	resp.State.SetAttribute(ctx, path.Root("logset_id"), logsetId)
	resp.State.SetAttribute(ctx, path.Root("topic_id"), topicId)
	resp.State.SetAttribute(ctx, path.Root("topic_name"), topicName)
	resp.State.SetAttribute(ctx, path.Root("area"), area)
	resp.State.SetAttribute(ctx, path.Root("domains"), domainList)
	resp.State.SetAttribute(ctx, path.Root("id"), req.ID)
}

func parseImportID(importID string) (string, string, error) {
	id := strings.Split(importID, ":")
	if len(id) != 2 {
		return "", "", fmt.Errorf("invalid import ID format, expected format: area,topicId (got: %s)", importID)
	}
	return id[0], id[1], nil
}

func (r *cdnRealTimeLogResource) createClsLogTopic(plan *cdnRealTimeLogResourceModel) (*tencentCloudCdnClient.CreateClsLogTopicResponse, error) {
	createClsLogTopicRequest := tencentCloudCdnClient.NewCreateClsLogTopicRequest()
	createClsLogTopicRequest.TopicName = common.StringPtr(plan.TopicName.ValueString())
	if plan.Area.ValueString() == "overseas" {
		createClsLogTopicRequest.LogsetId = common.StringPtr("236cc811-584f-4927-b64f-63c72e97b433")
	} else {
		createClsLogTopicRequest.LogsetId = common.StringPtr("c6955839-de16-430b-b987-abfa9ab7bedd")
	}

	var domainAreaConfigs []*tencentCloudCdnClient.DomainAreaConfig
	for _, domain := range plan.Domains.Elements() {
		domainStr := domain.(types.String).ValueString()
		domainAreaConfigs = append(domainAreaConfigs, &tencentCloudCdnClient.DomainAreaConfig{
			Domain: common.StringPtr(domainStr),
			Area:   []*string{common.StringPtr(plan.Area.ValueString())},
		})
	}

	createClsLogTopicRequest.DomainAreaConfigs = domainAreaConfigs

	var createClsLogTopicResponse *tencentCloudCdnClient.CreateClsLogTopicResponse
	createClsLogTopic := func() error {
		resp, err := r.client.CreateClsLogTopic(createClsLogTopicRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}
		createClsLogTopicResponse = resp
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(createClsLogTopic, reconnectBackoff)
	if err != nil {
		return nil, err
	}

	return createClsLogTopicResponse, nil
}

func (r *cdnRealTimeLogResource) updateClsLogTopic(state *cdnRealTimeLogResourceModel, plan *cdnRealTimeLogResourceModel) error {
	manageClsTopicDomainsRequest := tencentCloudCdnClient.NewManageClsTopicDomainsRequest()
	manageClsTopicDomainsRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	manageClsTopicDomainsRequest.LogsetId = common.StringPtr(getLogsetId(plan.Area.ValueString()))
	manageClsTopicDomainsRequest.Channel = common.StringPtr("cdn")

	var domainAreaConfigs []*tencentCloudCdnClient.DomainAreaConfig
	for _, domain := range plan.Domains.Elements() {
		domainStr := domain.(types.String).ValueString()
		domainAreaConfigs = append(domainAreaConfigs, &tencentCloudCdnClient.DomainAreaConfig{
			Domain: common.StringPtr(domainStr),
			Area:   []*string{common.StringPtr(plan.Area.ValueString())},
		})
	}
	manageClsTopicDomainsRequest.DomainAreaConfigs = domainAreaConfigs

	manageClsLogTopic := func() error {
		_, err := r.client.ManageClsTopicDomains(manageClsTopicDomainsRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
					return err
				}
				return backoff.Permanent(err)
			}
			return err
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	if err := backoff.Retry(manageClsLogTopic, reconnectBackoff); err != nil {
		return err
	}
	return nil
}

func getLogsetId(area string) string {
	if area == "overseas" {
		return "236cc811-584f-4927-b64f-63c72e97b433"
	}
	return "c6955839-de16-430b-b987-abfa9ab7bedd"
}
