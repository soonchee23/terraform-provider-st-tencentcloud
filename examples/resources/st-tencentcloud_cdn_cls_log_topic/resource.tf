resource "st-tencentcloud_cdn_cls_log_topic" "cls_log_topic" {
  logset_id = "xxxxxx-xxxxx-4927-xxxxx-63c7xxxxxx"
  topic_name = "cdn_cls_log_topic"
  domain_area_configs = [
    {
      domain = "static2.sige-test5.com",
      area   = "overseas"
    },
    {
      domain = "www1.sige-test6.com",
      area   = "overseas"
    },
  ]
}
