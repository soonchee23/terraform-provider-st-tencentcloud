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
