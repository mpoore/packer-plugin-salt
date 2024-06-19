packer {
  required_plugins {
    salt = {
      version = ">= 0.1.2"
      source  = "github.com/mpoore/salt"
    }
  }
}

variable "topping" {
  type    = string
  default = "mushroom"
}

source "docker" "example" {
  image       = "mpoore/salt-example:latest"
  export_path = "packer_example"
  run_command = ["-d", "-i", "-t", "--entrypoint=/bin/bash", "{{.Image}}"]
}

build {
  sources = [
    "source.docker.example"
  ]

  provisioner "salt" {
    state_files      = [ "example.sls" ]
    environment_vars = [ "TOPPINGS=${ var.topping }" ]
  }
}