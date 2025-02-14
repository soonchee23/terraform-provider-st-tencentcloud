resource "st-tencentcloud_cam_policy" "name" {
  user_id           = 123456789999
  attached_policies = ["QcloudAAFullAccess", "QcloudCamFullAccess"]
}
