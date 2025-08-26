// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/plugin"

	salt "github.com/mpoore/packer-plugin-salt/provisioner/salt"
	"github.com/mpoore/packer-plugin-salt/version"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterProvisioner(plugin.DEFAULT_NAME, new(salt.Provisioner))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
