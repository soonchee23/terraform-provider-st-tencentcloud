resource "st-tencentcloud_cdn_domain_config" "cdn_conditional_origin" {
  domain = "public1.sige-test3.com"
  origin {
    origin_list = ["47.239.216.145"]
    origin_type = "ip"
    conditional_origin_rules {
      rule_type  = "directory"
      origin     = ["public-324.oss-cn-hongkong.aliyuns.com"]
      rule_paths = ["/oss"]
    }
    rewrite_urls {
      rule_type  = "file"
      origin     = ["public-324.oss-cn-hongkong.qaliyunr.com"]
      rule_paths = ["jpg"]
    }
  }
}
