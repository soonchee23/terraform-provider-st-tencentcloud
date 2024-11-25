Terraform Custom Provider for Tencent Cloud
===========================================

This Terraform custom provider is designed for own use case scenario.

Supported Versions
------------------

| Terraform version | minimum provider version |maxmimum provider version
| ---- | ---- | ----|
| >= 1.3.x	| 0.1.0	| latest |

Requirements
------------

-	[Terraform](https://www.terraform.io/downloads.html) 1.3.x
-	[Go](https://golang.org/doc/install) 1.19 (to build the provider plugin)

Local Installation
------------------

1. Run make file `make install-local-custom-provider` to install the provider under ~/.terraform.d/plugins.

2. The provider source should be change to the path that configured in the *Makefile*:

    ```
    terraform {
      required_providers {
        st-tencentcloud = {
          source = "example.local/myklst/st-tencentcloud"
        }
      }
    }

    provider "st-tencentcloud" {
      region = "ap-hongkong"
    }
    ```

Why Custom Provider
-------------------

This custom provider exists due to some of the resources and data sources in the
official Tencent Cloud Terraform provider may not fulfill the requirements of some
scenario. The reason behind every resources and data sources are stated as below:

### Resources

- **st-tencentcloud_cam_user_group_attachment**

  The official TencentCloud Terraform provider's resource
  [*tencentcloud_cam_group_membership*](https://registry.terraform.io/providers/tencentcloudstack/tencentcloud/latest/docs/resources/cam_group_membership)
  will remove all other attached users for the target group, which may cause a
  problem where Terraform may delete those users attached outside from Terraform.

- **st-tencentcloud_enable_mfa_device**

  The official TencentCloud Terraform provider does not have
  the resource to enforce MFA for login.

- **st-tencentcloud_cdn_conditional_origin**

  The official TencentCloud Terraform provider's resource  [*tencentcloud_cdn_domain*](https://registry.terraform.io/providers/tencentcloudstack/tencentcloud/latest/docs/resources/cdn_domain)
  does not support adding path based origin rule and path rule in CDN.

### Data Sources

- **st-tencentcloud_clb_load_balancers**

  - The official TencentCloud Terraform provider's data source
    [*tencentcloud_clb_instances*](https://registry.terraform.io/providers/tencentcloudstack/tencentcloud/latest/docs/data-sources/clb_instances)
    do not support filtering load balancers with tags.

  - Added client_config block to allow overriding the Provider configuration.

- **st-tencentcloud_cdn_domains**

  - The official TencentCloud Terraform provider's data source
  [*tencentcloud_cdn_domains*](https://registry.terraform.io/providers/tencentcloudstack/tencentcloud/latest/docs/data-sources/cdn_domains)
  do no support querying CDN Domains from other account. This is solved by adding
  client_config block to allow overriding the Provider configuration.

References
----------

- Website: https://www.terraform.io
- Terraform Plugin Framework: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework
- Tencent Cloud official Terraform provider: https://github.com/tencentcloudstack/terraform-provider-tencentcloud
