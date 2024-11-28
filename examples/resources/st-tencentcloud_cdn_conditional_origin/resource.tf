resource "st-tencentcloud_cdn_conditional_origin" "cdn_conditional_origin" {
  domain = "public.example.com"

  rule {
    origin    = "example.oss-cn-hongkong.aliyuncs.com"
    rule_type = "directory"
    rule_path = "/example"
  }

  rule {
    origin    = "example.oss-cn-hongkong.aliyuncs.com"
    rule_type = "file"
    rule_path = "jpg"
  }

  rule {
    origin    = "example.oss-cn-hongkong.aliyuncs.com"
    rule_type = "path"
    rule_path = "/example/oren.jpg"
  }

  rule {
    origin    = "google.com"
    rule_type = "index"
    rule_path = "/"
  }
}
