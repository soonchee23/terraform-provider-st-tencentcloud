package tencentcloud

// https://github.com/TencentCloud/tencentcloud-sdk-go/blob/master/tencentcloud/clb/v20180317/errors.go
const (
	ERR_CLB_DRY_RUN_OPERATION     = "DryRunOperation"
	ERR_CLB_RESOURCE_IN_USE       = "ResourceInUse"
	ERR_CLB_RESOURCE_INSUFFICIENT = "ResourceInsufficient"
	ERR_INTERNAL_ERROR            = "InternalServerError"
	ERR_REQUEST_LIMIT_EXCEEDED    = "RequestLimitExceeded"
)

func isAbleToRetry(errCode string) bool {
	switch errCode {
	case
		ERR_CLB_DRY_RUN_OPERATION,
		ERR_CLB_RESOURCE_IN_USE,
		ERR_CLB_RESOURCE_INSUFFICIENT,
		ERR_INTERNAL_ERROR,
		ERR_REQUEST_LIMIT_EXCEEDED:
		return true
	default:
		return false
	}
	// return false
}
