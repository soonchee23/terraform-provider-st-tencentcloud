terraform {
  required_providers {
    st-tencentcloud = {
      source = "myklst/st-tencentcloud"
    }
  }
}

provider "st-tencentcloud" {
  region = "ap-hongkong"
}
