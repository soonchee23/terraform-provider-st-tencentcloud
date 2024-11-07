---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "st-tencentcloud_cdn_domain_config Resource - st-tencentcloud"
subcategory: ""
description: |-
  Provides a TencentCloud Path-Based Rule resource.
---

# st-tencentcloud_cdn_domain_config (Resource)

Provides a TencentCloud Path-Based Rule resource.

## Example Usage

```terraform
resource "st-tencentcloud_cdn_domain_config" "cdn_path_based_rule" {
  domain = var.tencent_cloud_cdn.domain_name

  origin {
    origin_list = ["47.239.216.145"]
    origin_type = "ip"

    conditional_origin_rules {
      rule_type  = "directory"
      origin     = "public-324.oss-cn-hongkong.aliyuns.com"
      rule_paths = ["/oss"]
    }

    rewrite_urls {
      path        = "/oss/*"
      server_name = "public-324.oss-cn-hongkong.aliyuns.com"
      origin_area = "CN"
      forward_uri = "/oss/$1"
      full_match  = false
      regex       = true
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `domain` (String) Domain name.

### Optional

- `origin` (Block List) Origin Configuration. (see [below for nested schema](#nestedblock--origin))

<a id="nestedblock--origin"></a>
### Nested Schema for `origin`

Required:

- `origin_list` (List of String) List of rule paths for origin.
- `origin_type` (String) Domain name.

Optional:

- `conditional_origin_rules` (Block List) Path-based Origin Configuration. (see [below for nested schema](#nestedblock--origin--conditional_origin_rules))
- `rewrite_urls` (Block List) Origin Path Rewrite Rule Configuration. (see [below for nested schema](#nestedblock--origin--rewrite_urls))
- `server_name` (String) Server name.

<a id="nestedblock--origin--conditional_origin_rules"></a>
### Nested Schema for `origin.conditional_origin_rules`

Required:

- `origin` (String) List of origin servers.
- `rule_paths` (List of String) List of rule paths for origin.
- `rule_type` (String) Type of the rule for origin.


<a id="nestedblock--origin--rewrite_urls"></a>
### Nested Schema for `origin.rewrite_urls`

Required:

- `forward_uri` (String) The URI path for the origin during path matching must start with “/” and cannot include parameters.
- `full_match` (Boolean) Ensures the entire string exactly matches a given pattern, with no extra characters.
- `origin_area` (String) The region of the origin site, supporting CN and OV.
- `path` (String) Matched URL paths only support URL paths and do not support parameters. The default is exact matching; when wildcard “*” matching is enabled, it supports up to 5 wildcards with a maximum length of 1024 characters.
- `regex` (Boolean) A pattern used to search, replace, or validate parts of a string.
- `server_name` (String) The Host header for the origin during path matching. If not specified, the default ServerName will be used.

Optional:

- `origin` (String) The origin site for path matching does not currently support COS sources with private read/write access. If not specified, the default origin site will be used.
- `request_headers` (Block Set) The alert notification methods. See the following Block alert_config. (see [below for nested schema](#nestedblock--origin--rewrite_urls--request_headers))

<a id="nestedblock--origin--rewrite_urls--request_headers"></a>
### Nested Schema for `origin.rewrite_urls.request_headers`

Optional:

- `header_mode` (String) The HTTP header configuration supports three methods: add, set, and del, which respectively mean adding a new header, setting (or modifying) an existing header, and deleting a header.
- `header_name` (String) HTTP header name.
- `header_value` (String) HTTP header value.

