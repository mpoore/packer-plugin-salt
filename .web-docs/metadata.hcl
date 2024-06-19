# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# For full specification on the configuration of this file visit:
# https://github.com/hashicorp/integration-template#metadata-configuration
integration {
  name = "Salt"
  description = "The Salt plugin enables users to apply Salt states to their Packer-built images for the purpose of further customizing them using Salt's powerful desired state automation."
  identifier = "packer/mpoore/salt"
  component {
    type = "provisioner"
    name = "Salt"
    slug = "salt"
  }
}