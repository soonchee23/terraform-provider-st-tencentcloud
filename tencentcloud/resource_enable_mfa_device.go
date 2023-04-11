package tencentcloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	tencentCloudCamClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cam/v20190116"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

var (
	_ resource.Resource              = &camMfaDeviceResource{}
	_ resource.ResourceWithConfigure = &camMfaDeviceResource{}
)

func NewCamMfaDeviceResource() resource.Resource {
	return &camMfaDeviceResource{}
}

type camMfaDeviceResource struct {
	client *tencentCloudCamClient.Client
}

type camMfaDeviceResourceModel struct {
	OpUin types.Int64 `tfsdk:"user_id"`
	Phone types.Int64 `tfsdk:"phone"`
}

func (r *camMfaDeviceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_enable_mfa_device"
}

func (r *camMfaDeviceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sets the MFA flag for a Tencent Cloud user.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.Int64Attribute{
				Description: "The user ID of a Tencent Cloud user.",
				Required:    true,
			},
			"phone": schema.Int64Attribute{
				Description: "Set phone as login flag.",
				Required:    true,
			},
		},
	}
}

func (r *camMfaDeviceResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).camClient
}

func (r *camMfaDeviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *camMfaDeviceResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setMfaFlag(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set MFA Login Flag",
			err.Error(),
		)
		return
	}

	state := &camMfaDeviceResourceModel{}
	state.OpUin = plan.OpUin
	state.Phone = plan.Phone

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camMfaDeviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *camMfaDeviceResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	describeSafeAuthFlagRequest := tencentCloudCamClient.NewDescribeSafeAuthFlagCollRequest()

	if !(state.OpUin.IsUnknown() || state.OpUin.IsNull()) {
		describeSafeAuthFlagRequest.SubUin = common.Uint64Ptr(uint64(state.OpUin.ValueInt64()))
	}

	describeSafeAuthFlag := func() error {
		_, err := r.client.DescribeSafeAuthFlagColl(describeSafeAuthFlagRequest)
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
	err := backoff.Retry(describeSafeAuthFlag, reconnectBackoff)
	if err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Describe Safe Auth Flag.",
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

func (r *camMfaDeviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan *camMfaDeviceResourceModel
	getPlanDiags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setMfaFlag(plan); err != nil {
		resp.Diagnostics.AddError(
			"[API ERROR] Failed to Set MFA Login Flag",
			err.Error(),
		)
		return
	}

	state := &camMfaDeviceResourceModel{}
	state.OpUin = plan.OpUin
	state.Phone = plan.Phone

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camMfaDeviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {

}

func (r *camMfaDeviceResource) setMfaFlag(plan *camMfaDeviceResourceModel) (err error) {
	setMfaFlagRequest := tencentCloudCamClient.NewSetMfaFlagRequest()
	setMfaFlagRequest.OpUin = common.Uint64Ptr(uint64(plan.OpUin.ValueInt64()))
	setMfaFlagRequest.LoginFlag = &tencentCloudCamClient.LoginActionMfaFlag{
		Phone: common.Uint64Ptr(uint64(plan.Phone.ValueInt64())),
	}

	setMfaFlag := func() error {
		if _, err := r.client.SetMfaFlag(setMfaFlagRequest); err != nil {
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
	return backoff.Retry(setMfaFlag, reconnectBackoff)
}
