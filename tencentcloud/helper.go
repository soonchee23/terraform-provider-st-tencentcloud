package tencentcloud

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	tencentCloudCdnClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tencentcloudCvmClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
)

////////////////////////////// Custom Framework Provider Related //////////////////////////////

func convertListStringtoListType(input []*string) basetypes.ListValue {
	newAttr := []attr.Value{}
	for _, x := range input {
		newAttr = append(newAttr, types.StringValue(*x))
	}
	return types.ListValueMust(types.StringType, newAttr)
}

////////////////////////////// TencentCloud Regions/Zones Related //////////////////////////////

func getZoneId(client *common.Client, zone string) (string, error) {
	cvmClient, err := tencentcloudCvmClient.NewClient(client.GetCredential(), client.GetRegion(), profile.NewClientProfile())
	if err != nil {
		return "", err
	}

	zones, err := cvmClient.DescribeZones(tencentcloudCvmClient.NewDescribeZonesRequest())
	if err != nil {
		return "", err
	}

	var zoneId string
	for _, z := range zones.Response.ZoneSet {
		if *z.Zone == zone {
			zoneId = *z.ZoneId
		}
	}
	return zoneId, nil
}

////////////////////////////// TencentCloud Client Related //////////////////////////////

type tencentcloudClientConfig struct {
	region     string
	credential *common.Credential
}

func initNewClient(providerConfig *common.Client, planConfig *clientConfig) (initClient bool, clientConfig *tencentcloudClientConfig) {
	initClient = false
	clientConfig = &tencentcloudClientConfig{
		credential: &common.Credential{},
	}
	region := planConfig.Region.ValueString()
	secretId := planConfig.SecretId.ValueString()
	secretKey := planConfig.SecretKey.ValueString()

	if region != "" || secretId != "" || secretKey != "" {
		initClient = true
	}

	if initClient {
		if region == "" {
			region = providerConfig.GetRegion()
		}
		if secretId == "" {
			secretId = providerConfig.GetCredential().GetSecretId()
		}
		if secretKey == "" {
			secretKey = providerConfig.GetCredential().GetSecretKey()
		}
		clientConfig.region = region
		clientConfig.credential = common.NewCredential(secretId, secretKey)
	}

	return
}

////////////////////////////// TencentCloud CDN Domain Related //////////////////////////////

func makeDomainFilter(key string, value string) *tencentCloudCdnClient.DomainFilter {
	return &tencentCloudCdnClient.DomainFilter{
		Name:  common.StringPtr(key),
		Value: common.StringPtrs([]string{value}),
	}
}
