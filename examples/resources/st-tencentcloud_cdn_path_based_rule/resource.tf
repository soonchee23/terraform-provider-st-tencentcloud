resource "st-tencentcloud_cdn_path_based_rule" "cdn_path_based_rule" {
  domain = "public1.sige-test3.com"

  origin {
    origin_list = ["47.239.216.145"]
    origin_type = "ip"

    path_based_origin_rule {
      rule_type  = "directory"
      origin     = ["public-324.oss-cn-hongkong.aliyuns.com"]
      rule_paths = ["/oss"]
    }

    path_rules {
      path        = "/oss/*"
      server_name = "sc-bucket-970991.oss-cn-hongkong.aliyuncs.com"
      origin_area = "CN"
      forward_uri = "/oss/$1"
      full_match  = false
      regex       = true
    }
  }
}
