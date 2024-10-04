resource "st-tencentcloud_cdn_path_based_rule" "cdn_path_based_rule" {
  domain = "public1.sige-test3.com"
  origin {
    origin_list = ["47.239.216.145"]
    origin_type = "ip"
    path_based_rule {
      rule_type  = "directory"
      origin     = ["public-324.oss-cn-hongkong.aliyuns.com"]
      rule_paths = ["/oss"]
    }
    path_based_rule {
      rule_type  = "file"
      origin     = ["public-324.oss-cn-hongkong.qaliyunr.com"]
      rule_paths = ["jpg"]
    }
  }
}
