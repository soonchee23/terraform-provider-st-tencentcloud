package tencentcloud

import (
	"context"
	"fmt"
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
	_ resource.Resource              = &cdnClsLogTopicResource{}
	_ resource.ResourceWithConfigure = &cdnClsLogTopicResource{}
)

func NewCdnClsLogTopicResource() resource.Resource {
	return &cdnClsLogTopicResource{}
}

type cdnClsLogTopicResource struct {
	client *tencentCloudCdnClient.Client
}

type cdnClsLogTopicResourceModel struct {
	TopicName         types.String         `tfsdk:"topic_name"`
	TopicId           types.String         `tfsdk:"topic_id"`
	LogsetId          types.String         `tfsdk:"logset_id"`
	DomainAreaConfigs []*domainAreaConfigs `tfsdk:"domain_area_configs"`
}

type domainAreaConfigs struct {
	Domain types.String `tfsdk:"domain"`
	Area   types.String `tfsdk:"area"`
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
			},
			"topic_id": schema.StringAttribute{
				Description: "The unique identifier of the CLS log topic, assigned by Tencent Cloud.",
				Computed:    true,
			},
			"logset_id": schema.StringAttribute{
				Description: "The ID of the CLS logset where the log topic is stored.",
				Required:    true,
			},
			"domain_area_configs": schema.ListNestedAttribute{
				Description: "List of domain and area configurations.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"domain": schema.StringAttribute{
							Description: "The domain name associated with this log configuration.",
							Required:    true,
						},
						"area": schema.StringAttribute{
							Description: "The area associated with the domain (only 'mainland' or 'overseas' allowed).",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("mainland", "overseas"),
							},
						},
					},
				},
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
	for _, config := range plan.DomainAreaConfigs {
		if _, exists := domainSet[config.Domain.ValueString()]; exists {
			resp.Diagnostics.AddError(
				"Duplicate Domain Found",
				fmt.Sprintf("The domain '%s' is defined multiple times in domain_area_configs.", config.Domain),
			)
			return
		}
		domainSet[config.Domain.ValueString()] = struct{}{}
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

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *cdnClsLogTopicResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *cdnClsLogTopicResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listCdnClsLogTopicRequest := tencentCloudCdnClient.NewListClsLogTopicsRequest()
	if !(state.TopicName.IsUnknown() || state.TopicName.IsNull()) {
		listCdnClsLogTopicRequest.Channel = common.StringPtr("cdn")
	}

	listCdnClsLogTopic := func() error {
		_, err := r.client.ListClsLogTopicsWithContext(ctx, listCdnClsLogTopicRequest)
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
	err := backoff.Retry(listCdnClsLogTopic, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe CDN CLS Log Topic.",
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

	if state.TopicId.IsNull() || state.LogsetId.IsNull() {
		resp.Diagnostics.AddError(
			"[UPDATE ERROR] Missing required parameters",
			"TopicId or LogsetId is missing from state, update cannot proceed.",
		)
		return
	}

	if err := r.updateClsLogTopic(&state, &plan); err != nil {
		resp.Diagnostics.AddError("[API ERROR]", err.Error())
		return
	}

	state.DomainAreaConfigs = plan.DomainAreaConfigs

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

	if state.TopicId.IsNull() || state.LogsetId.IsNull() {
		resp.Diagnostics.AddError(
			"[DELETE ERROR] Missing required parameters",
			"TopicId or LogsetId is missing from state, deletion cannot proceed.",
		)
		return
	}

	deleteClsLogTopicRequest := tencentCloudCdnClient.NewDeleteClsLogTopicRequest()
	deleteClsLogTopicRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	deleteClsLogTopicRequest.LogsetId = common.StringPtr(state.LogsetId.ValueString())

	_, err := r.client.DeleteClsLogTopic(deleteClsLogTopicRequest)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Delete CLS Log Topic.",
			err.Error(),
		)
		return
	}
}

func (r *cdnClsLogTopicResource) createClsLogTopic(plan *cdnClsLogTopicResourceModel) (*tencentCloudCdnClient.CreateClsLogTopicResponse, error) {
	createClsLogTopicRequest := tencentCloudCdnClient.NewCreateClsLogTopicRequest()
	createClsLogTopicRequest.TopicName = common.StringPtr(plan.TopicName.ValueString())
	createClsLogTopicRequest.LogsetId = common.StringPtr(plan.LogsetId.ValueString())

	var domainAreaConfigs []*tencentCloudCdnClient.DomainAreaConfig
	for _, domainAreaConfig := range plan.DomainAreaConfigs {
		domainAreaConfigs = append(domainAreaConfigs, &tencentCloudCdnClient.DomainAreaConfig{
			Domain: common.StringPtr(domainAreaConfig.Domain.ValueString()),
			Area:   []*string{common.StringPtr(domainAreaConfig.Area.ValueString())},
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

func (r *cdnClsLogTopicResource) updateClsLogTopic(state *cdnClsLogTopicResourceModel, plan *cdnClsLogTopicResourceModel) error {
	manageClsTopicDomainsRequest := tencentCloudCdnClient.NewManageClsTopicDomainsRequest()
	manageClsTopicDomainsRequest.TopicId = common.StringPtr(state.TopicId.ValueString())
	manageClsTopicDomainsRequest.LogsetId = common.StringPtr(state.LogsetId.ValueString())
	manageClsTopicDomainsRequest.Channel = common.StringPtr("cdn")

	var domainAreaConfigs []*tencentCloudCdnClient.DomainAreaConfig
	for _, domainAreaConfig := range plan.DomainAreaConfigs {
		domainAreaConfigs = append(domainAreaConfigs, &tencentCloudCdnClient.DomainAreaConfig{
			Domain: common.StringPtr(domainAreaConfig.Domain.ValueString()),
			Area:   []*string{common.StringPtr(domainAreaConfig.Area.ValueString())},
		})
	}
	manageClsTopicDomainsRequest.DomainAreaConfigs = domainAreaConfigs

	_, err := r.client.ManageClsTopicDomains(manageClsTopicDomainsRequest)
	if err != nil {
		return fmt.Errorf("[API ERROR] Failed to Update CLS Log Topic Domains: %w", err)
	}

	return nil
}
