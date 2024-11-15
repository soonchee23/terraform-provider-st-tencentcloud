resource "st-tencentcloud_path_based_origin_rule" "cdn_path_based_origin_rule" {
  domain = "public.example.com"

  origin {
    origin_list = ["google.com"]
    origin_type = "ip"
    origin_pull_protocol = "follow"

    path_based_origin_rule {
      rule_type  = "directory"
      origin     = "public-324.oss-cn-hongkong.aliyuns.com"
      rule_paths = ["/oss"]
    }

    rewrite_path_rule {
      path        = "/oss/*"
      server_name = "public-324.oss-cn-hongkong.aliyuns.com"
      origin_area = "CN"
      forward_uri = "/oss/$1"
      full_match  = false
      regex       = true
    }
  }
}
