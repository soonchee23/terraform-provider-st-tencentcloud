resource "st-tencentcloud_cdn_real_time_log" "cls_log_topic" {
  topic_name = "cdn_real_time_log"
  area       = "overseas"
  domains = [
    "a.example.com",
    "b.example.com",
  ]
}
