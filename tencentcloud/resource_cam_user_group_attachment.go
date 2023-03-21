package tencentcloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"

	tencentCloudCamClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cam/v20190116"
)

var (
	_ resource.Resource              = &camUserGroupAttachmentResource{}
	_ resource.ResourceWithConfigure = &camUserGroupAttachmentResource{}
)

func NewCamUserGroupAttachmentResource() resource.Resource {
	return &camUserGroupAttachmentResource{}
}

type camUserGroupAttachmentResource struct {
	client *tencentCloudCamClient.Client
}

type camUserGroupAttachmentResourceModel struct {
	GroupID types.Int64 `tfsdk:"group_id"`
	UserID  types.Int64 `tfsdk:"user_id"`
}

func (r *camUserGroupAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cam_user_group_attachment"
}

func (r *camUserGroupAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a TencentCloud CAM Group Membership resource.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.Int64Attribute{
				Description: "The ID of CAM group.",
				Required:    true,
			},
			"user_id": schema.Int64Attribute{
				Description: "The user ID of CAM group member.",
				Required:    true,
			},
		},
	}
}

func (r *camUserGroupAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).camClient
}

func (r *camUserGroupAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *camUserGroupAttachmentResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.addUserToGroup(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Add User to Group.",
			err.Error(),
		)
		return
	}

	state := &camUserGroupAttachmentResourceModel{}
	state.GroupID = plan.GroupID
	state.UserID = plan.UserID

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camUserGroupAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *camUserGroupAttachmentResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	listUserForGroupRequest := tencentCloudCamClient.NewListUsersForGroupRequest()

	if !(state.GroupID.IsUnknown() || state.GroupID.IsNull()) {
		listUserForGroupRequest.GroupId = common.Uint64Ptr(uint64(state.GroupID.ValueInt64()))
	}

	readUserForGroup := func() error {
		listUserForGroupResponse, err := r.client.ListUsersForGroup(listUserForGroupRequest)
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

		for _, user := range listUserForGroupResponse.Response.UserInfo {
			if *user.Uid == uint64(state.UserID.ValueInt64()) {
				return nil
			}
		}
		state.UserID = types.Int64Value(0)
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(readUserForGroup, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Add User to Group",
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

func (r *camUserGroupAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *camUserGroupAttachmentResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.addUserToGroup(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Add User to Group.",
			err.Error(),
		)
		return
	}

	state := &camUserGroupAttachmentResourceModel{}
	state.GroupID = plan.GroupID
	state.UserID = plan.UserID

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camUserGroupAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *camUserGroupAttachmentResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removeUserFromGroupRequest := tencentCloudCamClient.NewRemoveUserFromGroupRequest()
	removeUserFromGroupRequest.Info = []*tencentCloudCamClient.GroupIdOfUidInfo{
		{
			Uid:     common.Uint64Ptr(uint64(state.UserID.ValueInt64())),
			GroupId: common.Uint64Ptr(uint64(state.GroupID.ValueInt64())),
		},
	}

	if _, err := r.client.RemoveUserFromGroup(removeUserFromGroupRequest); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Remove User from Group",
			err.Error(),
		)
		return
	}
}

func (r *camUserGroupAttachmentResource) addUserToGroup(plan *camUserGroupAttachmentResourceModel) (err error) {
	addUserToGroupRequest := tencentCloudCamClient.NewAddUserToGroupRequest()
	addUserToGroupRequest.Info = []*tencentCloudCamClient.GroupIdOfUidInfo{
		{
			Uid:     common.Uint64Ptr(uint64(plan.UserID.ValueInt64())),
			GroupId: common.Uint64Ptr(uint64(plan.GroupID.ValueInt64())),
		},
	}

	addUserToGroup := func() error {
		if _, err := r.client.AddUserToGroup(addUserToGroupRequest); err != nil {
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
	return backoff.Retry(addUserToGroup, reconnectBackoff)
}
