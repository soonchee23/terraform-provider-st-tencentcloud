resource "st-tencentcloud_cdn_cls_log_topic" "cls_log_topic" {
  topic_name = "cdn_cls_log_topic"
  area       = "overseas"
  domains = [
    "a.example.com",
    "b.example.com",
  ]
}
