package tencentcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"

	tencentCloudCamClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cam/v20190116"
)

const (
	// Number of 30 indicates the character length of neccessary policy keyword
	// such as "Version" and "Statement" and some JSON symbols ({}, []).
	policyKeywordLength = 30
	policyMaxLength     = 6144
)

var (
	_ resource.Resource              = &camPolicyResource{}
	_ resource.ResourceWithConfigure = &camPolicyResource{}
	//_ resource.ResourceWithImportState = &camPolicyResource{}
)

func NewCamPolicyResource() resource.Resource {
	return &camPolicyResource{}
}

type camPolicyResource struct {
	client *tencentCloudCamClient.Client
}

type camPolicyResourceModel struct {
	UserID                 types.Int64     `tfsdk:"user_id"`
	AttachedPolicies       types.List      `tfsdk:"attached_policies"`
	AttachedPoliciesDetail []*policyDetail `tfsdk:"attached_policies_detail"`
	CombinedPolicesDetail  []*policyDetail `tfsdk:"combined_policies_detail"`
}

type policyDetail struct {
	PolicyName     types.String `tfsdk:"policy_name"`
	PolicyDocument types.String `tfsdk:"policy_document"`
}

func (r *camPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cam_policy"
}

func (r *camPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a CAM Policy resource that manages policy content " +
			"exceeding character limits by splitting it into smaller segments. " +
			"These segments are combined to form a complete policy attached to " +
			"the user. However, the policy that exceed the maximum length of a " +
			"policy, they will be attached directly to the user.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.Int64Attribute{
				Description: "The id of the CAM user that attached to the policy.",
				Required:    true,
			},
			"attached_policies": schema.ListAttribute{
				Description: "The CAM policies to attach to the user.",
				Required:    true,
				ElementType: types.StringType,
			},
			"attached_policies_detail": schema.ListNestedAttribute{
				Description: "A list of policies. Used to compare whether policy has been changed outside of Terraform",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"policy_name": schema.StringAttribute{
							Description: "The policy name.",
							Computed:    true,
						},
						"policy_document": schema.StringAttribute{
							Description: "The policy document of the CAM policy.",
							Computed:    true,
						},
					},
				},
			},
			"combined_policies_detail": schema.ListNestedAttribute{
				Description: "A list of combined policies that are attached to users.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"policy_name": schema.StringAttribute{
							Description: "The policy name.",
							Computed:    true,
						},
						"policy_document": schema.StringAttribute{
							Description: "The policy document of the CAM policy.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (r *camPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(tencentCloudClients).camClient
}

func (r *camPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan *camPolicyResourceModel
	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	combinedPolicies, attachedPolicies, errors := r.createPolicy(ctx, plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Create the Policy.",
		errors,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	state := &camPolicyResourceModel{}
	state.UserID = plan.UserID
	state.AttachedPolicies = plan.AttachedPolicies
	state.AttachedPoliciesDetail = attachedPolicies
	state.CombinedPolicesDetail = combinedPolicies

	attachPolicyToUserErr := r.attachPolicyToUser(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Attach Policy to User.",
		attachPolicyToUserErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create policy are not expected to have not found warning.
	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserID),
		readCombinedPolicyNotExistErr,
		"",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserID),
		readCombinedPolicyErr,
		"",
	)

	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state *camPolicyResourceModel
	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// This state will be using to compare with the current state.
	var oriState *camPolicyResourceModel
	getOriStateDiags := req.State.Get(ctx, &oriState)
	resp.Diagnostics.Append(getOriStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserID),
		readCombinedPolicyNotExistErr,
		"The combined policies may be deleted due to human mistake or API error, will trigger update to recreate the combined policy:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserID),
		readCombinedPolicyErr,
		"",
	)

	// Set state so that Terraform will trigger update if there are changes in state.
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.WarningsCount() > 0 || resp.Diagnostics.HasError() {
		return
	}

	// If the attached policy not found, it should return warning instead of error
	// because there is no ways to get plan configuration in Read() function to
	// indicate user had removed the non existed policies from the input.
	readAttachedPolicyNotExistErr, readAttachedPolicyErr := r.readAttachedPolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Failed to Read Attached Policies for %v: Policy Not Found!", state.UserID),
		readAttachedPolicyNotExistErr,
		"The policy that will be used to combine policies had been removed on TencentCloud, next apply with update will prompt error:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Unexpected Error!", state.UserID),
		readAttachedPolicyErr,
		"",
	)

	// Set state so that Terraform will trigger update if there are changes in state.
	setStateDiags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.WarningsCount() > 0 || resp.Diagnostics.HasError() {
		return
	}

	compareAttachedPoliciesErr := r.checkPoliciesDrift(state, oriState)
	addDiagnostics(
		&resp.Diagnostics,
		"warning",
		fmt.Sprintf("[API WARNING] Policy Drift Detected for %v.", state.UserID),
		[]error{compareAttachedPoliciesErr},
		"This resource will be updated in the next terraform apply.",
	)

	setStateDiags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state *camPolicyResourceModel
	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	getStateDiags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(getStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Make sure each of the attached policies are exist before removing the combined
	// policies.
	readAttachedPolicyNotExistErr, readAttachedPolicyErr := r.readAttachedPolicy(ctx, plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Policy Not Found!", state.UserID),
		readAttachedPolicyNotExistErr,
		"The policy that will be used to combine policies had been removed on TencentCloud:",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Attached Policies for %v: Unexpected Error!", state.UserID),
		readAttachedPolicyErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	removePolicyErr := r.removePolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Remove Policies for %v: Unexpected Error!", state.UserID),
		removePolicyErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	state.CombinedPolicesDetail = nil
	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	combinedPolicies, attachedPolicies, errors := r.createPolicy(ctx, plan)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Create the Policy.",
		errors,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	state.UserID = plan.UserID
	state.AttachedPolicies = plan.AttachedPolicies
	state.AttachedPoliciesDetail = attachedPolicies
	state.CombinedPolicesDetail = combinedPolicies

	attachPolicyToUserErr := r.attachPolicyToUser(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		"[API ERROR] Failed to Attach Policy to User.",
		attachPolicyToUserErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create policy are not expected to have not found warning.
	readCombinedPolicyNotExistErr, readCombinedPolicyErr := r.readCombinedPolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Policy Not Found!", state.UserID),
		readCombinedPolicyNotExistErr,
		"",
	)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Read Combined Policies for %v: Unexpected Error!", state.UserID),
		readCombinedPolicyErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}

	setStateDiags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *camPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state *camPolicyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	removePolicyUnexpectedErr := r.removePolicy(ctx, state)
	addDiagnostics(
		&resp.Diagnostics,
		"error",
		fmt.Sprintf("[API ERROR] Failed to Remove Policies for %v: Unexpected Error!", state.UserID),
		removePolicyUnexpectedErr,
		"",
	)
	if resp.Diagnostics.HasError() {
		return
	}
}

// createPolicy will create the combined policy and return the attached policies
// details to be saved in state for comparing in Read() function.
//
// Parameters:
//   - ctx: Context.
//   - plan: Terraform plan configurations.
//
// Returns:
//   - combinedPoliciesDetail: The combined policies detail to be recorded in state file.
//   - attachedPoliciesDetail: The attached policies detail to be recorded in state file.
//   - errList: List of errors, return nil if no errors.
func (r *camPolicyResource) createPolicy(ctx context.Context, plan *camPolicyResourceModel) (combinedPoliciesDetail []*policyDetail, attachedPoliciesDetail []*policyDetail, errList []error) {
	var policies []string
	plan.AttachedPolicies.ElementsAs(ctx, &policies, false)
	combinedPolicyDocuments, excludedPolicies, attachedPoliciesDetail, errList := r.combinePolicyDocument(ctx, policies)
	if errList != nil {
		return nil, nil, errList
	}

	createPolicy := func() error {
		for i, policy := range combinedPolicyDocuments {
			policyName := fmt.Sprintf("%s-%d", plan.UserID, i+1)

			createPolicyRequest := tencentCloudCamClient.NewCreatePolicyRequest()
			createPolicyRequest.PolicyName = common.StringPtr(policyName)
			createPolicyRequest.PolicyDocument = common.StringPtr(policy)

			if _, err := r.client.CreatePolicy(createPolicyRequest); err != nil {
				return handleAPIError(err)
			}
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(createPolicy, reconnectBackoff)

	if err != nil {
		return nil, nil, []error{err}
	}

	for i, policies := range combinedPolicyDocuments {
		policyName := fmt.Sprintf("%s-%d", plan.UserID, i+1)

		combinedPoliciesDetail = append(combinedPoliciesDetail, &policyDetail{
			PolicyName:     types.StringValue(policyName),
			PolicyDocument: types.StringValue(policies),
		})
	}

	// These policies will be attached directly to the user since splitting the
	// policy "statement" will be hitting the limitation of "maximum number of
	// attached policies" easily.
	combinedPoliciesDetail = append(combinedPoliciesDetail, excludedPolicies...)

	return combinedPoliciesDetail, attachedPoliciesDetail, nil
}

// combinePolicyDocument combine the policy with custom logic.
//
// Parameters:
//   - ctx: Context.
//   - attachedPolicies: List of user attached policies to be combined.
//
// Returns:
//   - combinedPolicyDocument: The completed policy document after combining attached policies.
//   - excludedPolicies: If the target policy exceeds maximum length, then do not combine the policy and return as excludedPolicies.
//   - attachedPoliciesDetail: The attached policies detail to be recorded in state file.
//   - errList: List of errors, return nil if no errors.
func (r *camPolicyResource) combinePolicyDocument(ctx context.Context, attachedPolicies []string) (combinedPolicyDocument []string, excludedPolicies []*policyDetail, attachedPoliciesDetail []*policyDetail, errList []error) {
	attachedPoliciesDetail, notExistErrList, unexpectedErrList := r.fetchPolicies(ctx, attachedPolicies, "", "")

	errList = append(errList, notExistErrList...)
	errList = append(errList, unexpectedErrList...)

	if len(errList) != 0 {
		return nil, nil, nil, errList
	}

	currentLength := 0
	currentPolicyStatement := ""
	appendedPolicyStatement := make([]string, 0)

	for _, attachedPolicy := range attachedPoliciesDetail {
		tempPolicyDocument := attachedPolicy.PolicyDocument.ValueString()
		// If the policy itself have more than 6144 characters, then skip the combine
		// policy part since splitting the policy "statement" will be hitting the
		// limitation of "maximum number of attached policies" easily.
		if len(tempPolicyDocument) > policyMaxLength {
			excludedPolicies = append(excludedPolicies, &policyDetail{
				PolicyName:     attachedPolicy.PolicyName,
				PolicyDocument: types.StringValue(tempPolicyDocument),
			})
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(tempPolicyDocument), &data); err != nil {
			errList = append(errList, err)
			return nil, nil, nil, errList
		}

		statementBytes, err := json.Marshal(data["statement"])
		if err != nil {
			errList = append(errList, err)
			return nil, nil, nil, errList
		}

		finalStatement := strings.Trim(string(statementBytes), "[]")
		currentLength += len(finalStatement)

		// Before further proceeding the current policy, we need to add a number
		// of 'policyKeywordLength' to simulate the total length of completed
		// policy to check whether it is already execeeded the max character
		// length of 6144.
		if (currentLength + policyKeywordLength) > policyMaxLength {
			currentPolicyStatement = strings.TrimSuffix(currentPolicyStatement, ",")
			appendedPolicyStatement = append(appendedPolicyStatement, currentPolicyStatement)
			currentPolicyStatement = finalStatement + ","
			currentLength = len(finalStatement)
		} else {
			currentPolicyStatement += finalStatement + ","
		}
	}

	if len(currentPolicyStatement) > 0 {
		currentPolicyStatement = strings.TrimSuffix(currentPolicyStatement, ",")
		appendedPolicyStatement = append(appendedPolicyStatement, currentPolicyStatement)
	}

	for _, policyStatement := range appendedPolicyStatement {
		combinedPolicyDocument = append(combinedPolicyDocument, fmt.Sprintf(`{"version":"2.0","statement":[%v]}`, policyStatement))
	}

	return combinedPolicyDocument, excludedPolicies, attachedPoliciesDetail, nil
}

// readCombinedPolicy will read the combined policy details.
//
// Parameters:
//   - ctx: Context.
//   - state: The state configurations, it will directly update the value of the struct since it is a pointer.
//
// Returns:
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return nil if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return nil if no errors.
func (r *camPolicyResource) readCombinedPolicy(ctx context.Context, state *camPolicyResourceModel) (notExistErrs, unexpectedErrs []error) {
	var policiesName []string
	for _, policy := range state.CombinedPolicesDetail {
		policiesName = append(policiesName, policy.PolicyName.ValueString())
	}

	policyDetails, notExistErrs, unexpectedErrs := r.fetchPolicies(ctx, policiesName, fmt.Sprintf("%v-", state.UserID), "Local")
	if len(unexpectedErrs) > 0 {
		return nil, unexpectedErrs
	}

	// If the combined policies not found from TencentCloud, that it might be deleted
	// from outside Terraform. Set the state to Unknown to trigger state changes
	// and Update() function.
	if len(notExistErrs) > 0 {
		// This is to ensure Update() is called.
		state.AttachedPolicies = types.ListNull(types.StringType)
	}

	state.CombinedPolicesDetail = policyDetails
	return notExistErrs, nil
}

// readAttachedPolicy will read the attached policy details.
//
// Parameters:
//   - ctx: Context.
//   - state: The state configurations, it will directly update the value of the struct since it is a pointer.
//
// Returns:
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return nil if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return nil if no errors.
func (r *camPolicyResource) readAttachedPolicy(ctx context.Context, state *camPolicyResourceModel) (notExistErrs, unexpectedErrs []error) {
	var policiesName []string
	for _, policyName := range state.AttachedPolicies.Elements() {
		policiesName = append(policiesName, strings.Trim(policyName.String(), "\""))
	}

	policyDetails, notExistErrs, unexpectedErrs := r.fetchPolicies(ctx, policiesName, "", "")
	if len(unexpectedErrs) > 0 {
		return nil, unexpectedErrs
	}

	// If the combined policies not found from TencentCloud, that it might be deleted
	// from outside Terraform. Set the state to Unknown to trigger state changes
	// and Update() function.
	if len(notExistErrs) > 0 {
		// This is to ensure Update() is called.
		state.AttachedPolicies = types.ListNull(types.StringType)
	}

	state.AttachedPoliciesDetail = policyDetails
	return notExistErrs, nil
}

// fetchPolicies retrieve policy document through TencentCloud SDK with backoff retry.
//
// Parameters:
//   - ctx: Context.
//   - policiesName: List of CAM policies name.
//
// Returns:
//   - policiesDetail: List of retrieved policies detail.
//   - notExistError: List of allowed not exist errors to be used as warning messages instead, return empty list if no errors.
//   - unexpectedError: List of unexpected errors to be used as normal error messages, return empty list if no errors.
func (r *camPolicyResource) fetchPolicies(ctx context.Context, policiesName []string, keyword string, scope string) (policiesDetail []*policyDetail, notExistError, unexpectedError []error) {
	getPolicyResponse := &tencentCloudCamClient.GetPolicyResponse{}

	policyIDMap, err := r.getAllPolicyIDs(ctx, keyword, scope)
	if err != nil {
		unexpectedError = append(unexpectedError, err)
		return
	}

	// Create a number of 10 channels to be ran concurrently, the number is
	// adjusted accordingly to TencentCloud API rate limit of 20QPS.
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, attachedPolicy := range policiesName {
		policyID, exists := policyIDMap[attachedPolicy]
		if !exists {
			notExistError = append(notExistError, fmt.Errorf("policy %v does not exist", attachedPolicy))
			continue
		}

		// Add new queue to WaitGroup.
		wg.Add(1)
		// Add new channel to indicate the current number of running
		// goroutine functions. It will block other channels when it
		// reaches the maximum number of available channels
		// as stated above.
		sem <- struct{}{}

		go func(policyID uint64, policyName string) {
			// Remove the queue from WaitGroup.
			defer wg.Done()
			// Release the channel.
			defer func() { <-sem }()

			getPolicyRequest := tencentCloudCamClient.NewGetPolicyRequest()
			getPolicyRequest.PolicyId = common.Uint64Ptr(policyID)

			getPolicy := func() error {
				getPolicyResponse, err = r.client.GetPolicyWithContext(ctx, getPolicyRequest)
				if err != nil {
					return handleAPIError(err)
				}
				return nil
			}

			reconnectBackoff := backoff.NewExponentialBackOff()
			reconnectBackoff.MaxElapsedTime = 30 * time.Second
			err = backoff.Retry(getPolicy, reconnectBackoff)

			// To prevent race conditions when appending variables
			// in the code below.
			mu.Lock()
			defer mu.Unlock()

			// Handle permanent error returned from API.
			if err != nil {
				switch err.(*errors.TencentCloudSDKError).Code {
				case "ResourceNotFound.PolicyIdNotFound":
					notExistError = append(notExistError, err)
				default:
					unexpectedError = append(unexpectedError, err)
				}
				wg.Wait()
				return
			}

			policiesDetail = append(policiesDetail, &policyDetail{
				PolicyName:     types.StringValue(*getPolicyResponse.Response.PolicyName),
				PolicyDocument: types.StringValue(*getPolicyResponse.Response.PolicyDocument),
			})

		}(policyID, attachedPolicy)
	}
	// Ensures all remaining queue from WaitGroup have finished running.
	wg.Wait()

	return
}

// checkPoliciesDrift compare the recorded AttachedPoliciesDetail documents with
// the latest CAM policy documents on TencentCloud, and trigger Update() if policy
// drift is detected.
//
// Parameters:
//   - newState: New attached policy details that returned from TencentCloud SDK.
//   - oriState: Original policy details that are recorded in Terraform state.
//
// Returns:
//   - error: The policy drifting error.
func (r *camPolicyResource) checkPoliciesDrift(newState, oriState *camPolicyResourceModel) error {
	var driftedPolicies []string

	for _, oldPolicyDetailState := range oriState.AttachedPoliciesDetail {
		for _, currPolicyDetailState := range newState.AttachedPoliciesDetail {
			if oldPolicyDetailState.PolicyName.String() == currPolicyDetailState.PolicyName.String() {
				if oldPolicyDetailState.PolicyDocument.String() != currPolicyDetailState.PolicyDocument.String() {
					driftedPolicies = append(driftedPolicies, oldPolicyDetailState.PolicyName.String())
				}
				break
			}
		}
	}

	if len(driftedPolicies) > 0 {
		// Set the state to trigger an update.
		newState.AttachedPolicies = types.ListNull(types.StringType)

		return fmt.Errorf(
			"the following policies documents had been changed since combining policies: [%s]",
			strings.Join(driftedPolicies, ", "),
		)
	}

	return nil
}

// removePolicy will detach and delete the combined policies from user.
//
// Parameters:
//   - ctx: Context.
//   - state: The recorded state configurations.
func (r *camPolicyResource) removePolicy(ctx context.Context, state *camPolicyResourceModel) (unexpectedError []error) {
	removePolicy := func() error {
		for _, combinedPolicy := range state.CombinedPolicesDetail {
			policyID, err := r.getPolicyID(ctx, combinedPolicy.PolicyName.ValueString())
			if err != nil {
				unexpectedError = append(unexpectedError, err)
				break
			}

			detachPolicyFromUserRequest := tencentCloudCamClient.NewDetachUserPolicyRequest()
			detachPolicyFromUserRequest.PolicyId = common.Uint64Ptr(policyID)
			detachPolicyFromUserRequest.DetachUin = common.Uint64Ptr(uint64(state.UserID.ValueInt64()))

			if _, err := r.client.DetachUserPolicyWithContext(ctx, detachPolicyFromUserRequest); err != nil {
				// TencentCloud SDK does not return error if
				// policy is already detached before calling
				return handleAPIError(err)
			}

			deletePolicyRequest := tencentCloudCamClient.NewDeletePolicyRequest()
			deletePolicyRequest.PolicyId = common.Uint64Ptrs([]uint64{policyID})

			if _, err := r.client.DeletePolicyWithContext(ctx, deletePolicyRequest); err != nil {
				// Ignore error where the policy had been deleted
				// as it is intended to delete the IAM policy.
				if err.(*errors.TencentCloudSDKError).Code != "InvalidParameter.PolicyIDNotExist" {
					return handleAPIError(err)
				}
			}
		}

		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(removePolicy, reconnectBackoff)
	if err != nil {
		return append(unexpectedError, err)
	}

	return nil
}

// attachPolicyToUser attach the CAM policy to user through TencentCloud SDK.
//
// Parameters:
//   - ctx: Context
//   - state: The recorded state configurations.
//
// Returns:
//   - err: Error.
func (r *camPolicyResource) attachPolicyToUser(ctx context.Context, state *camPolicyResourceModel) (unexpectedError []error) {
	attachPolicyToUser := func() error {
		for _, combinedPolicy := range state.CombinedPolicesDetail {
			policyID, err := r.getPolicyID(ctx, combinedPolicy.PolicyName.ValueString())
			if err != nil {
				unexpectedError = append(unexpectedError, err)
				break
			}

			attachPolicyToUserRequest := tencentCloudCamClient.NewAttachUserPolicyRequest()
			attachPolicyToUserRequest.PolicyId = common.Uint64Ptr(policyID)
			attachPolicyToUserRequest.AttachUin = common.Uint64Ptr(uint64(state.UserID.ValueInt64()))

			if _, err := r.client.AttachUserPolicyWithContext(ctx, attachPolicyToUserRequest); err != nil {
				return handleAPIError(err)
			}
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	if err := backoff.Retry(attachPolicyToUser, reconnectBackoff); err != nil {
		unexpectedError = append(unexpectedError, err)
	}

	return unexpectedError
}

func handleAPIError(err error) error {
	if _t, ok := err.(*errors.TencentCloudSDKError); ok {
		if isRetryableErrCode(_t.Code) {
			return err
		} else if isRetryableErrMessage(_t.Message){
			return err
		} else {
			return backoff.Permanent(err)
		}
	} else {
		return err
	}
}

// getAllPolicyIDs gets all the CAM policy's ID through TencentCloud SDK based on keyword and scope.
//
// Parameters:
//   - ctx: Context
//   - keyword: The keyword to filter the query.
//   - scope: The type of query to be filtered
//
// Returns:
//   - policyIDMap: Map of policies name to id.
func (r *camPolicyResource) getAllPolicyIDs(ctx context.Context, keyword string, scope string) (map[string]uint64, error) {
	policyIDMap := make(map[string]uint64)
	var err error

	pageSize := uint64(200) // Max allowed per request
	page := uint64(1)       // Start from page 1

	listPoliciesRequest := tencentCloudCamClient.NewListPoliciesRequest()
	if keyword != "" {
		listPoliciesRequest.Keyword = &keyword
	}
	if scope != "" {
		listPoliciesRequest.Scope = &scope
	}

	for {
		var listPoliciesResponse *tencentCloudCamClient.ListPoliciesResponse

		listPolicies := func() error {
			listPoliciesRequest.Rp = &pageSize
			listPoliciesRequest.Page = &page

			listPoliciesResponse, err = r.client.ListPoliciesWithContext(ctx, listPoliciesRequest)
			if err != nil {
				return handleAPIError(err)
			}
			return nil
		}

		reconnectBackoff := backoff.NewExponentialBackOff()
		reconnectBackoff.MaxElapsedTime = 30 * time.Second
		err = backoff.Retry(listPolicies, reconnectBackoff)
		if err != nil {
			return nil, err
		}

		if listPoliciesResponse == nil || listPoliciesResponse.Response == nil || len(listPoliciesResponse.Response.List) == 0 {
			break
		}

		for _, policyObj := range listPoliciesResponse.Response.List {
			policyIDMap[*policyObj.PolicyName] = *policyObj.PolicyId
		}

		// Stop if we've already requested all available policies
		if page*pageSize >= *listPoliciesResponse.Response.TotalNum {
			break
		}

		page++
	}

	return policyIDMap, nil
}

// getPolicyID gets the CAM policy's ID through TencentCloud SDK.
//
// Parameters:
//   - ctx: Context
//   - policyName: The name of the requested policy's ID.
//
// Returns:
//   - policyID: The policy ID of the policyName.
func (r *camPolicyResource) getPolicyID(ctx context.Context, policyName string) (policyID uint64, err error) {
	var listPoliciesResponse *tencentCloudCamClient.ListPoliciesResponse

	listPolicies := func() error {
		listPoliciesRequest := tencentCloudCamClient.NewListPoliciesRequest()
		listPoliciesRequest.Keyword = &policyName
		listPoliciesResponse, err = r.client.ListPoliciesWithContext(ctx, listPoliciesRequest)
		if err != nil {
			return handleAPIError(err)
		}
		return nil
	}

	reconnectBackoff := backoff.NewExponentialBackOff()
	reconnectBackoff.MaxElapsedTime = 30 * time.Second
	err = backoff.Retry(listPolicies, reconnectBackoff)

	if err != nil {
		return 0, err
	}

	// This for loop is because the list may return policies who share a common name.
	for _, policyObj := range listPoliciesResponse.Response.List {
		if *policyObj.PolicyName == policyName {
			policyID = *policyObj.PolicyId
			return
		}
	}

	return
}

func addDiagnostics(diags *diag.Diagnostics, severity string, title string, errors []error, extraMessage string) {
	var combinedMessages string
	validErrors := 0

	for _, err := range errors {
		if err != nil {
			combinedMessages += fmt.Sprintf("%v\n", err)
			validErrors++
		}
	}

	if validErrors == 0 {
		return
	}

	var message string
	if extraMessage != "" {
		message = fmt.Sprintf("%s\n%s", extraMessage, combinedMessages)
	} else {
		message = combinedMessages
	}

	switch severity {
	case "warning":
		diags.AddWarning(title, message)
	case "error":
		diags.AddError(title, message)
	default:
		// Handle unknown severity if needed
	}
}
