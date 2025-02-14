package tencentcloud

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tencentCloudCdnClient "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

var (
	_ datasource.DataSource              = &cdnDomainsDataSource{}
	_ datasource.DataSourceWithConfigure = &cdnDomainsDataSource{}
)

func NewCdnDomainsDataSource() datasource.DataSource {
	return &cdnDomainsDataSource{}
}

type cdnDomainsDataSource struct {
	client *tencentCloudCdnClient.Client
}

type cdnDomainsDataSourceModel struct {
	ClientConfig       *clientConfig      `tfsdk:"client_config"`
	Domain             types.String       `tfsdk:"domain"`
	ServiceType        types.String       `tfsdk:"service_type"`
	FullUrlCache       types.Bool         `tfsdk:"full_url_cache"`
	OriginPullProtocol types.String       `tfsdk:"origin_pull_protocol"`
	HttpsSwitch        types.String       `tfsdk:"https_switch"`
	DomainList         []*cdnDomainDetail `tfsdk:"domain_list"`
}

type cdnDomainDetail struct {
	Id                types.String              `tfsdk:"id"`
	Domain            types.String              `tfsdk:"domain"`
	Cname             types.String              `tfsdk:"cname"`
	Status            types.String              `tfsdk:"status"`
	CreateTime        types.String              `tfsdk:"create_time"`
	UpdateTime        types.String              `tfsdk:"update_time"`
	ServiceType       types.String              `tfsdk:"service_type"`
	Area              types.String              `tfsdk:"area"`
	ProjectId         types.Int64               `tfsdk:"project_id"`
	FullUrlCache      types.Bool                `tfsdk:"full_url_cache"`
	RangeOriginSwitch types.String              `tfsdk:"range_origin_switch"`
	RequestHeader     []*cdnDomainRequestHeader `tfsdk:"request_header"`
	RuleCache         []*cdnDomainRuleCache     `tfsdk:"rule_cache"`
	Origin            []*cdnDomainOrigin        `tfsdk:"origin"`
	HttpsConfig       []*cdnDomainHttpsConfig   `tfsdk:"https_config"`
	Tags              types.Map                 `tfsdk:"tags"`
}

type cdnDomainRequestHeader struct {
	Switch      types.String           `tfsdk:"switch"`
	HeaderRules []*cdnDomainHeaderRule `tfsdk:"header_rules"`
}

type cdnDomainHeaderRule struct {
	HeaderMode  types.String `tfsdk:"header_mode"`
	HeaderName  types.String `tfsdk:"header_name"`
	HeaderValue types.String `tfsdk:"header_value"`
	RuleType    types.String `tfsdk:"rule_type"`
	RulePaths   types.List   `tfsdk:"rule_paths"`
}

type cdnDomainRuleCache struct {
	RulePaths          types.List   `tfsdk:"rule_paths"`
	RuleType           types.String `tfsdk:"rule_type"`
	Switch             types.String `tfsdk:"switch"`
	CacheTime          types.Int64  `tfsdk:"cache_time"`
	CompareMaxAge      types.String `tfsdk:"compare_max_age"`
	IgnoreCacheControl types.String `tfsdk:"ignore_cache_control"`
	IgnoreSetCookie    types.String `tfsdk:"ignore_set_cookie"`
	NoCacheSwitch      types.String `tfsdk:"no_cache_switch"`
	ReValidate         types.String `tfsdk:"re_validate"`
	FollowOriginSwitch types.String `tfsdk:"follow_origin_switch"`
}

type cdnDomainOrigin struct {
	OriginType         types.String `tfsdk:"origin_type"`
	OriginList         types.List   `tfsdk:"origin_list"`
	BackupOriginType   types.String `tfsdk:"backup_origin_type"`
	BackupOriginList   types.List   `tfsdk:"backup_origin_list"`
	BackupServerName   types.String `tfsdk:"backup_server_name"`
	ServerName         types.String `tfsdk:"server_name"`
	CosPrivateAccess   types.String `tfsdk:"cos_private_access"`
	OriginPullProtocol types.String `tfsdk:"origin_pull_protocol"`
}

type cdnDomainHttpsConfig struct {
	HttpsSwitch        types.String `tfsdk:"https_switch"`
	Http2Switch        types.String `tfsdk:"http2_switch"`
	OcspStaplingSwitch types.String `tfsdk:"ocsp_stapling_switch"`
	SpdySwitch         types.String `tfsdk:"spdy_switch"`
	VerifyClient       types.String `tfsdk:"verify_client"`
}

func (d *cdnDomainsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cdn_domains"
}

func (d *cdnDomainsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to query the detail information of CDN domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Description: "Acceleration domain name.",
				Optional:    true,
			},
			"service_type": schema.StringAttribute{
				Description: "Service type of acceleration domain name. The available value include `web`, `download` and `media`.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("web", "download", "media"),
				},
			},
			"full_url_cache": schema.BoolAttribute{
				Description: "Whether to enable full-path cache.",
				Optional:    true,
			},
			"origin_pull_protocol": schema.StringAttribute{
				Description: "Origin-pull protocol configuration. Valid values: `http`, `https` and `follow`.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("http", "https", "follow"),
				},
			},
			"https_switch": schema.StringAttribute{
				Description: "HTTPS configuration. Valid values: `on`, `off` and `processing`.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("on", "off", "processing"),
				},
			},
			"domain_list": schema.ListNestedAttribute{
				Description: "An information list of cdn domain. Each element contains the following attributes:",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Domain name ID.",
							Computed:    true,
						},
						"domain": schema.StringAttribute{
							Description: "Acceleration domain name.",
							Computed:    true,
						},
						"cname": schema.StringAttribute{
							Description: "CNAME address of domain name.",
							Computed:    true,
						},
						"status": schema.StringAttribute{
							Description: "Acceleration service status.",
							Computed:    true,
						},
						"create_time": schema.StringAttribute{
							Description: "Domain name creation time.",
							Computed:    true,
						},
						"update_time": schema.StringAttribute{
							Description: "Last modified time of domain name.",
							Computed:    true,
						},
						"service_type": schema.StringAttribute{
							Description: "Service type of acceleration domain name.",
							Computed:    true,
						},
						"area": schema.StringAttribute{
							Description: "Acceleration region.",
							Computed:    true,
						},
						"project_id": schema.Int64Attribute{
							Description: "The project CDN belongs to.",
							Computed:    true,
						},
						"full_url_cache": schema.BoolAttribute{
							Description: "Whether to enable full-path cache.",
							Computed:    true,
						},
						"range_origin_switch": schema.StringAttribute{
							Description: "Sharding back to source configuration switch.",
							Computed:    true,
						},
						"request_header": schema.ListNestedAttribute{
							Description: "Request header configuration.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"switch": schema.StringAttribute{
										Description: "Custom request header configuration switch.",
										Computed:    true,
									},
									"header_rules": schema.ListNestedAttribute{
										Description: "Custom request header configuration rules.",
										Computed:    true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"header_mode": schema.StringAttribute{
													Description: "Http header setting method.",
													Computed:    true,
												},
												"header_name": schema.StringAttribute{
													Description: "Http header name.",
													Computed:    true,
												},
												"header_value": schema.StringAttribute{
													Description: "Http header value.",
													Computed:    true,
												},
												"rule_type": schema.StringAttribute{
													Description: "Rule type.",
													Computed:    true,
												},
												"rule_paths": schema.ListAttribute{
													Description: "Rule paths.",
													Computed:    true,
													ElementType: types.StringType,
												},
											},
										},
									},
								},
							},
						},
						"rule_cache": schema.ListNestedAttribute{
							Description: "Advanced path cache configuration.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"rule_paths": schema.ListAttribute{
										Description: "Rule paths.",
										Computed:    true,
										ElementType: types.StringType,
									},
									"rule_type": schema.StringAttribute{
										Description: "Rule type.",
										Computed:    true,
									},
									"switch": schema.StringAttribute{
										Description: "Cache configuration switch.",
										Computed:    true,
									},
									"cache_time": schema.Int64Attribute{
										Description: "Cache expiration time setting, the unit is second.",
										Computed:    true,
									},
									"compare_max_age": schema.StringAttribute{
										Description: "Advanced cache expiration configuration.",
										Computed:    true,
									},
									"ignore_cache_control": schema.StringAttribute{
										Description: "Force caching. After opening, the no-store and no-cache resources returned by the origin site will also be cached in accordance with the CacheRules rules.",
										Computed:    true,
									},
									"ignore_set_cookie": schema.StringAttribute{
										Description: "Ignore the Set-Cookie header of the origin site.",
										Computed:    true,
									},
									"no_cache_switch": schema.StringAttribute{
										Description: "Cache configuration switch.",
										Computed:    true,
									},
									"re_validate": schema.StringAttribute{
										Description: "Always check back to origin.",
										Computed:    true,
									},
									"follow_origin_switch": schema.StringAttribute{
										Description: "Follow the source station configuration switch.",
										Computed:    true,
									},
								},
							},
						},
						"origin": schema.ListNestedAttribute{
							Description: "Origin server configuration.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"origin_type": schema.StringAttribute{
										Description: "Master origin server type.",
										Computed:    true,
									},
									"origin_list": schema.ListAttribute{
										Description: "Master origin server list.",
										Computed:    true,
										ElementType: types.StringType,
									},
									"backup_origin_type": schema.StringAttribute{
										Description: "Backup origin server type.",
										Computed:    true,
									},
									"backup_origin_list": schema.ListAttribute{
										Description: "Backup origin server list.",
										Computed:    true,
										ElementType: types.StringType,
									},
									"backup_server_name": schema.StringAttribute{
										Description: "Host header used when accessing the backup origin server. If left empty, the ServerName of master origin server will be used by default.",
										Computed:    true,
									},
									"server_name": schema.StringAttribute{
										Description: "Host header used when accessing the master origin server. If left empty, the acceleration domain name will be used by default.",
										Computed:    true,
									},
									"cos_private_access": schema.StringAttribute{
										Description: "When OriginType is COS, you can specify if access to private buckets is allowed.",
										Computed:    true,
									},
									"origin_pull_protocol": schema.StringAttribute{
										Description: "Origin-pull protocol configuration.",
										Computed:    true,
									},
								},
							},
						},
						"https_config": schema.ListNestedAttribute{
							Description: "HTTPS acceleration configuration. It's a list and consist of at most one item.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"https_switch": schema.StringAttribute{
										Description: "HTTPS configuration switch.",
										Computed:    true,
									},
									"http2_switch": schema.StringAttribute{
										Description: "HTTP2 configuration switch.",
										Computed:    true,
									},
									"ocsp_stapling_switch": schema.StringAttribute{
										Description: "OCSP configuration switch.",
										Computed:    true,
									},
									"spdy_switch": schema.StringAttribute{
										Description: "Spdy configuration switch.",
										Computed:    true,
									},
									"verify_client": schema.StringAttribute{
										Description: "Client certificate authentication feature.",
										Computed:    true,
									},
								},
							},
						},
						"tags": schema.MapAttribute{
							Description: "",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"client_config": schema.SingleNestedBlock{
				Description: "Config to override default client created in Provider. " +
					"This block will not be recorded in state file.",
				Attributes: map[string]schema.Attribute{
					"region": schema.StringAttribute{
						Description: "The region of the CLBs. Default to use region " +
							"configured in the provider.",
						Optional: true,
					},
					"secret_id": schema.StringAttribute{
						Description: "The secret id that have permissions to list " +
							"CLBs. Default to use secret id configured in the provider.",
						Optional: true,
					},
					"secret_key": schema.StringAttribute{
						Description: "The secret key that have permissions to list " +
							"CLBs. Default to use secret key configured in the provider.",
						Optional: true,
					},
				},
			},
		},
	}
}

func (d *cdnDomainsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(tencentCloudClients).cdnClient
}

func (d *cdnDomainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var plan, state *cdnDomainsDataSourceModel
	getPlanDiags := req.Config.Get(ctx, &plan)
	resp.Diagnostics.Append(getPlanDiags...)
	if getPlanDiags.HasError() {
		return
	}

	if plan.ClientConfig == nil {
		plan.ClientConfig = &clientConfig{}
	}

	initClient, clientConfig := initNewClient(&d.client.Client, plan.ClientConfig)
	if initClient {
		var err error
		d.client, err = tencentCloudCdnClient.NewClient(clientConfig.credential, clientConfig.region, profile.NewClientProfile())
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Reinitialize Tencent Cloud Load Balancers API Client",
				"An unexpected error occurred when creating the Tencent Cloud Load Balancers API client. "+
					"If the error is not clear, please contact the provider developers.\n\n"+
					"Tencent Cloud Load Balancers Client Error: "+err.Error(),
			)
			return
		}
	}

	request := tencentCloudCdnClient.NewDescribeDomainsConfigRequest()
	if plan.Domain.ValueString() != "" {
		request.Filters = append(request.Filters, makeDomainFilter("domain", plan.Domain.ValueString()))
	}
	if plan.ServiceType.ValueString() != "" {
		request.Filters = append(request.Filters, makeDomainFilter("serviceType", plan.ServiceType.ValueString()))
	}
	if plan.HttpsSwitch.ValueString() != "" {
		request.Filters = append(request.Filters, makeDomainFilter("httpsSwitch", plan.HttpsSwitch.ValueString()))
	}
	if plan.OriginPullProtocol.ValueString() != "" {
		request.Filters = append(request.Filters, makeDomainFilter("originPullProtocol", plan.OriginPullProtocol.ValueString()))
	}
	if !plan.FullUrlCache.IsUnknown() && !plan.FullUrlCache.IsNull() {
		var fullUrlCacheStr string
		if plan.FullUrlCache.ValueBool() {
			fullUrlCacheStr = "on"
		} else {
			fullUrlCacheStr = "off"
		}
		request.Filters = append(request.Filters, makeDomainFilter("fullUrlCache", fullUrlCacheStr))
	}

	response := tencentCloudCdnClient.NewDescribeDomainsConfigResponse()
	describeCdnDomain := func() error {
		var err error
		response, err = d.client.DescribeDomainsConfig(request)
		if err != nil {
			if terr, ok := err.(*errors.TencentCloudSDKError); ok {
				if isRetryableErrCode(terr.GetCode()) {
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
		return
	}

	state = &cdnDomainsDataSourceModel{}
	state.DomainList = make([]*cdnDomainDetail, 0, len(response.Response.Domains))
	for _, detailDomain := range response.Response.Domains {
		var fullUrlCache bool
		if detailDomain.CacheKey != nil && *detailDomain.CacheKey.FullUrlCache == "on" {
			fullUrlCache = true
		}

		requestHeaders := make([]*cdnDomainRequestHeader, 0, 1)
		requestHeader := &cdnDomainRequestHeader{
			Switch:      types.StringValue(*detailDomain.RequestHeader.Switch),
			HeaderRules: make([]*cdnDomainHeaderRule, 0, len(detailDomain.RequestHeader.HeaderRules)),
		}
		for _, value := range detailDomain.RequestHeader.HeaderRules {
			requestHeader.HeaderRules = append(requestHeader.HeaderRules, &cdnDomainHeaderRule{
				HeaderMode:  types.StringValue(*value.HeaderMode),
				HeaderName:  types.StringValue(*value.HeaderName),
				HeaderValue: types.StringValue(*value.HeaderValue),
				RuleType:    types.StringValue(*value.RuleType),
				RulePaths:   convertListStringtoListType(value.RulePaths),
			})
		}
		requestHeaders = append(requestHeaders, requestHeader)

		ruleCaches := make([]*cdnDomainRuleCache, 0, len(detailDomain.Cache.RuleCache))
		for _, value := range detailDomain.Cache.RuleCache {
			ruleCaches = append(ruleCaches, &cdnDomainRuleCache{
				RulePaths:          convertListStringtoListType(value.RulePaths),
				RuleType:           types.StringValue(*value.RuleType),
				Switch:             types.StringValue(*value.CacheConfig.Cache.Switch),
				CacheTime:          types.Int64Value(*value.CacheConfig.Cache.CacheTime),
				CompareMaxAge:      types.StringValue(*value.CacheConfig.Cache.CompareMaxAge),
				IgnoreCacheControl: types.StringValue(*value.CacheConfig.Cache.IgnoreCacheControl),
				IgnoreSetCookie:    types.StringValue(*value.CacheConfig.Cache.IgnoreSetCookie),
				NoCacheSwitch:      types.StringValue(*value.CacheConfig.NoCache.Switch),
				ReValidate:         types.StringValue(*value.CacheConfig.NoCache.Revalidate),
				FollowOriginSwitch: types.StringValue(*value.CacheConfig.FollowOrigin.Switch),
			})
		}

		origins := make([]*cdnDomainOrigin, 0, 1)
		origin := &cdnDomainOrigin{}
		origin.OriginType = types.StringValue(*detailDomain.Origin.OriginType)
		origin.OriginList = convertListStringtoListType(detailDomain.Origin.Origins)
		if detailDomain.Origin.BackupOriginType != nil {
			origin.BackupOriginType = types.StringValue(*detailDomain.Origin.BackupOriginType)
			origin.BackupOriginList = convertListStringtoListType(detailDomain.Origin.BackupOrigins)
			origin.BackupServerName = types.StringValue(*detailDomain.Origin.BackupServerName)
		} else {
			origin.BackupOriginList = types.ListNull(types.StringType)
		}
		origin.ServerName = types.StringValue(*detailDomain.Origin.ServerName)
		origin.CosPrivateAccess = types.StringValue(*detailDomain.Origin.CosPrivateAccess)
		origin.OriginPullProtocol = types.StringValue(*detailDomain.Origin.OriginPullProtocol)
		origins = append(origins, origin)

		httpsConfigs := make([]*cdnDomainHttpsConfig, 0, 1)
		if detailDomain.Https != nil {
			httpsConfigs = append(httpsConfigs, &cdnDomainHttpsConfig{
				HttpsSwitch:        types.StringValue(*detailDomain.Https.Switch),
				Http2Switch:        types.StringValue(*detailDomain.Https.Http2),
				OcspStaplingSwitch: types.StringValue(*detailDomain.Https.OcspStapling),
				SpdySwitch:         types.StringValue(*detailDomain.Https.Spdy),
				VerifyClient:       types.StringValue(*detailDomain.Https.VerifyClient),
			})
		}

		var cdnTags types.Map
		if detailDomain.Tag == nil || len(detailDomain.Tag) < 1 {
			cdnTags = types.MapNull(types.StringType)
		} else {
			// Convert API output Tags to Go map
			cdnTagsMap := make(map[string]attr.Value)
			for _, value := range detailDomain.Tag {
				cdnTagsMap[*value.TagKey] = types.StringValue(*value.TagValue)
			}
			cdnTags = types.MapValueMust(types.StringType, cdnTagsMap)
		}

		state.DomainList = append(state.DomainList, &cdnDomainDetail{
			Id:                types.StringValue(*detailDomain.ResourceId),
			Domain:            types.StringValue(*detailDomain.Domain),
			Cname:             types.StringValue(*detailDomain.Cname),
			Status:            types.StringValue(*detailDomain.Status),
			CreateTime:        types.StringValue(*detailDomain.CreateTime),
			UpdateTime:        types.StringValue(*detailDomain.UpdateTime),
			ServiceType:       types.StringValue(*detailDomain.ServiceType),
			Area:              types.StringValue(*detailDomain.Area),
			ProjectId:         types.Int64Value(*detailDomain.ProjectId),
			FullUrlCache:      types.BoolValue(fullUrlCache),
			RangeOriginSwitch: types.StringValue(*detailDomain.RangeOriginPull.Switch),
			RequestHeader:     requestHeaders,
			RuleCache:         ruleCaches,
			Origin:            origins,
			HttpsConfig:       httpsConfigs,
			Tags:              cdnTags,
		})
	}

	state.Domain = plan.Domain
	state.FullUrlCache = plan.FullUrlCache
	state.HttpsSwitch = plan.HttpsSwitch
	state.OriginPullProtocol = plan.OriginPullProtocol
	state.ServiceType = plan.ServiceType

	setStateDiags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(setStateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
