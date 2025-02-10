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
	_ resource.Resource               = &cdnClsLogTopicResource{}
	_ resource.ResourceWithConfigure  = &cdnClsLogTopicResource{}
	_ resource.ResourceWithModifyPlan = &cdnClsLogTopicResource{}
)

func NewCdnClsLogTopicResource() resource.Resource {
	return &cdnClsLogTopicResource{}
}

type cdnClsLogTopicResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnClsLogTopicResourceModel struct {
	TopicName types.String `tfsdk:"topic_name"`
	Area      types.String `tfsdk:"area"`
	TopicId   types.String `tfsdk:"topic_id"`
	LogsetId  types.String `tfsdk:"logset_id"`
	Domains   types.List   `tfsdk:"domains"`
}

func (r *cdnClsLogTopicResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_cls_log_topic"
}

func (r *cdnClsLogTopicResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tencent Cloud CDN CLS log topic resource.",
		Attributes: map[string]schema.Attribute{
			"topic_name": schema.StringAttribute{
				Description: "The name of the CLS log topic to be created.",
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
			"topic_id": schema.StringAttribute{
				Description: "The unique identifier of the CLS log topic, assigned by Tencent Cloud.",
				Computed:    true,
			},
			"logset_id": schema.StringAttribute{
				Description: "The ID of the CLS logset where the log topic is stored.",
				Computed:    true,
			},
			"domains": schema.ListAttribute{
				Description: "The domain name associated with this log configuration.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *cdnClsLogTopicResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (r *cdnClsLogTopicResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If the entire plan is null, the resource is planned for destruction.
	if req.Plan.Raw.IsNull() {
		fmt.Println("Plan is null; skipping ModifyPlan.")
		return
	}

	var plan cdnClsLogTopicResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check whether the domain is defined multiple times
	domainSet := make(map[string]struct{})
	for _, domain := range plan.Domains.Elements() {
		domainStr := domain.(types.String).ValueString()

		if _, exists := domainSet[domainStr]; exists {
			resp.Diagnostics.AddError(
				"Duplicate Domain Found",
				fmt.Sprintf("The domain '%s' is defined multiple times in the domains list.", domainStr),
			)
			return
		}
		domainSet[domainStr] = struct{}{}
	}
}

func (r *cdnClsLogTopicResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, state cdnClsLogTopicResourceModel
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
	if plan.Area.ValueString() == "overseas" {
		state.LogsetId = types.StringValue("236cc811-584f-4927-b64f-63c72e97b433")
	} else {
		state.LogsetId = types.StringValue("c6955839-de16-430b-b987-abfa9ab7bedd")
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnClsLogTopicResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cdnClsLogTopicResourceModel
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

	listClsTopicDomains := func() error {
		respData, err := r.client.ListClsTopicDomainsWithContext(ctx, listClsTopicDomainsRequest)
		if err != nil {
			if t, ok := err.(*errors.TencentCloudSDKError); ok {
				if isAbleToRetry(t.GetCode()) {
					return err
				} else {
					return backoff.Permanent(err)
				}
			}
			return err
		}

		if len(respData.Response.DomainAreaConfigs) == 0 {
			resp.Diagnostics.AddWarning("[WARNING] Empty DomainAreaConfigs", "No domain configurations were returned by the API.")
			state.Domains = types.ListNull(types.StringType)
			return nil
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
			return fmt.Errorf("[ERROR] Failed to convert domains to list: %v", diags)
		}

		state.Domains = domainsList
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(listClsTopicDomains, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError("[API ERROR] Failed to Describe CDN CLS Topic Domains.", err.Error())
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnClsLogTopicResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var logsetId, topicId string
	var err error

	logsetId, topicId, err = parseImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	listClsTopicDomainsRequest := tencentCloudCdnClient.NewListClsTopicDomainsRequest()
	listClsTopicDomainsRequest.LogsetId = common.StringPtr(logsetId)
	listClsTopicDomainsRequest.TopicId = common.StringPtr(topicId)
	listClsTopicDomainsRequest.Channel = common.StringPtr("cdn")

	listClsTopicDomainsResponse, err := r.client.ListClsTopicDomainsWithContext(ctx, listClsTopicDomainsRequest)
	if err != nil {
		resp.Diagnostics.AddError("Failed to retrieve CLS log topic details", err.Error())
		return
	}

	if listClsTopicDomainsResponse.Response == nil {
		resp.Diagnostics.AddError("Empty response from Tencent Cloud", "Response object is nil.")
		return
	}

	var topicName string
	var area *string
	var domainList types.List

	if listClsTopicDomainsResponse.Response.TopicName != nil {
		topicName = *listClsTopicDomainsResponse.Response.TopicName
	}

	switch logsetId {
	case "236cc811-584f-4927-b64f-63c72e97b433":
		area = common.StringPtr("overseas")
	case "c6955839-de16-430b-b987-abfa9ab7bedd":
		area = common.StringPtr("mainland")
	default:
		area = nil
	}

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
		return "", "", fmt.Errorf("invalid import ID format, expected format: logsetId:topicId (got: %s)", importID)
	}
	return id[0], id[1], nil
}

func (r *cdnClsLogTopicResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cdnClsLogTopicResourceModel
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
		resp.Diagnostics.AddError("[API ERROR]", err.Error())
		return
	}

	state.Domains = plan.Domains

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnClsLogTopicResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cdnClsLogTopicResourceModel
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

func (r *cdnClsLogTopicResource) createClsLogTopic(plan *cdnClsLogTopicResourceModel) (*tencentCloudCdnClient.CreateClsLogTopicResponse, error) {
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
				} else {
					return backoff.Permanent(err)
				}
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

func (r *cdnClsLogTopicResource) updateClsLogTopic(state *cdnClsLogTopicResourceModel, plan *cdnClsLogTopicResourceModel) (err error) {
	manageClsTopicDomainsRequest := tencentCloudCdnClient.NewManageClsTopicDomainsRequest()
	manageClsTopicDomainsRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	if plan.Area.ValueString() == "overseas" {
		manageClsTopicDomainsRequest.LogsetId = common.StringPtr("236cc811-584f-4927-b64f-63c72e97b433")
	} else {
		manageClsTopicDomainsRequest.LogsetId = common.StringPtr("c6955839-de16-430b-b987-abfa9ab7bedd")
	}
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

	updateClsLogTopic := func() error {
		if _, err := r.client.ManageClsTopicDomains(manageClsTopicDomainsRequest); err != nil {
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
	return backoff.Retry(updateClsLogTopic, reconnectBackoff)
}
