// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config
//go:generate packer-sdc struct-markdown

package salt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

var saltProvisionerMap = map[string]string{
	"configStagingDirL": "/tmp/packer-provisioner-salt",
	"configStagingDirW": "C:/Windows/Temp/packer-provisioner-salt",
	"cmdTemplateW":      "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -Command {%s}",
	"cmdCreateDirL":     "mkdir -p '%s'",
	"cmdCreateDirW":     "PS: New-Item -ItemType Directory -Path %s -Force",
	"cmdDeleteDirL":     "rm -rf '%s'",
	"cmdDeleteDirW":     "PS: Remove-Item -Recurse -Force %s",
	"cmdSaltCallL":      "salt-call --local --file-root=%s state.apply %s",
	"cmdSaltCallW":      "salt-call --local --file-root=%s state.apply %s",
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	ctx                 interpolate.Context
	// The type of OS that the workload is using. By setting this value to `true` then
	// it is assumed that the target OS is Windows based. By default this setting is `false`.
	IsWindows bool `mapstructure:"windows"`
	// The state files to be applied by Salt. These files must exist on
	// your local system where Packer is executing.
	StateFiles []string `mapstructure:"state_files"`
	// The directory where files will be uploaded. Packer requires write
	// permissions in this directory.
	StagingDir string `mapstructure:"staging_directory"`
	// If set to `true`, the content of the `staging_directory` will be removed after
	// applying Salt states. By default this is set to `false`.
	CleanStagingDir bool `mapstructure:"clean_staging_directory"`
	// If set to `true`, the command to execute Salt will be prefixed by `sudo`
	// on Linux / Unix based systems. By default this is set to `false`.
	UseSudo bool `mapstructure:"use_sudo"`
}

type Provisioner struct {
	config        Config
	stateFiles    []string
	generatedData map[string]interface{}
}

func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec { return p.config.FlatMapstructure().HCL2Spec() }

func (p *Provisioner) Prepare(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         "salt",
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return err
	}

	// Reset the state.
	p.stateFiles = make([]string, 0, len(p.config.StateFiles))

	// Defaults
	if p.config.StagingDir == "" {
		p.config.StagingDir = p.getMapValue(ui, "configStagingDir")
	}

	// Validation
	var errs *packersdk.MultiError

	// Check that state_files is specified
	if len(p.config.StateFiles) == 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("The parameter state_files must be specified"))
	}

	for _, stateFile := range p.config.StateFiles {
		if err := validateFileConfig(stateFile, "state_files", true); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		} else {
			p.stateFiles = append(p.stateFiles, stateFile)
		}
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

func (p *Provisioner) Provision(ctx context.Context, ui packersdk.Ui, comm packersdk.Communicator, generatedData map[string]interface{}) error {
	ui.Say("Provisioning with Salt...")
	p.generatedData = generatedData

	ui.Message("Creating Salt staging directory...")
	if err := p.createDir(ui, comm, p.config.StagingDir); err != nil {
		return fmt.Errorf("Error creating staging directory: %s", err)
	}

	if err := p.uploadStateFiles(ui, comm); err != nil {
		return err
	}

	if err := p.executeSalt(ui, comm); err != nil {
		return fmt.Errorf("Error executing Salt: %s", err)
	}

	if p.config.CleanStagingDir {
		ui.Message("Removing staging directory...")
		if err := p.removeDir(ui, comm, p.config.StagingDir); err != nil {
			return fmt.Errorf("Error removing staging directory: %s", err)
		}
	}
	return nil
}

func (p *Provisioner) uploadStateFiles(ui packersdk.Ui, comm packersdk.Communicator) error {
	for _, stateFile := range p.stateFiles {
		if err := p.uploadStateFile(ui, comm, stateFile); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) uploadStateFile(ui packersdk.Ui, comm packersdk.Communicator, stateFile string) error {
	localStateFile, _ := filepath.Abs(stateFile)
	ui.Message(fmt.Sprintf("Uploading state file: %s", localStateFile))

	remoteDir := filepath.ToSlash(filepath.Join(p.config.StagingDir, filepath.Dir(stateFile)))
	remoteStateFile := filepath.ToSlash(filepath.Join(p.config.StagingDir, stateFile))

	if err := p.createDir(ui, comm, remoteDir); err != nil {
		return fmt.Errorf("Error uploading state file: %s [%s]", localStateFile, err)
	}

	if err := p.uploadFile(ui, comm, remoteStateFile, localStateFile); err != nil {
		return fmt.Errorf("Error uploading state file: %s [%s]", localStateFile, err)
	}

	return nil
}

func (p *Provisioner) executeSalt(ui packersdk.Ui, comm packersdk.Communicator) error {
	// Fetch external dependencies
	for _, stateFile := range p.stateFiles {
		if err := p.executeSaltState(ui, comm, stateFile); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) executeSaltState(
	ui packersdk.Ui, comm packersdk.Communicator, stateFile string,
) error {
	ctx := context.TODO()
	stateName := strings.ReplaceAll(stateFile, ".sls", "")
	command := p.getMapValue(ui, "cmdSaltCall")
	command = fmt.Sprintf(command, p.config.StagingDir, stateName)
	ui.Message(fmt.Sprintf("Executing Salt: %s", command))
	cmd := &packersdk.RemoteCmd{
		Command: command,
	}
	if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
		return err
	}
	if cmd.ExitStatus() != 0 {
		if cmd.ExitStatus() == 127 {
			return fmt.Errorf("%s could not be found. Verify that it is available on the\n"+
				"PATH after connecting to the machine.",
				command)
		}

		return fmt.Errorf("Non-zero exit status: %d", cmd.ExitStatus())
	}
	return nil
}

func validateDirConfig(path string, config string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %s is invalid: %s", config, path, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s: %s must point to a directory", config, path)
	}
	return nil
}

func validateFileConfig(name string, config string, req bool) error {
	if req {
		if name == "" {
			return fmt.Errorf("%s must be specified.", config)
		}
	}
	info, err := os.Stat(name)
	if err != nil {
		return fmt.Errorf("%s: %s is invalid: %s", config, name, err)
	} else if info.IsDir() {
		return fmt.Errorf("%s: %s must point to a file", config, name)
	}
	return nil
}

func (p *Provisioner) uploadFile(ui packersdk.Ui, comm packersdk.Communicator, dst, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("Error opening: %s", err)
	}
	defer f.Close()

	if err = comm.Upload(dst, f, nil); err != nil {
		return fmt.Errorf("Error uploading %s: %s", src, err)
	}
	return nil
}

func (p *Provisioner) createDir(ui packersdk.Ui, comm packersdk.Communicator, dir string) error {
	ctx := context.TODO()
	command := p.getMapValue(ui, "cmdCreateDir")
	cmd := &packersdk.RemoteCmd{
		Command: fmt.Sprintf(command, dir),
	}

	ui.Message(fmt.Sprintf("Creating directory: %s", dir))
	if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
		return err
	}

	if cmd.ExitStatus() != 0 {
		return fmt.Errorf("Non-zero exit status. See output above for more information.")
	}
	return nil
}

func (p *Provisioner) removeDir(ui packersdk.Ui, comm packersdk.Communicator, dir string) error {
	ctx := context.TODO()
	command := p.getMapValue(ui, "cmdDeleteDir")
	cmd := &packersdk.RemoteCmd{
		Command: fmt.Sprintf(command, dir),
	}

	ui.Message(fmt.Sprintf("Removing directory: %s", dir))
	if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
		return err
	}

	if cmd.ExitStatus() != 0 {
		return fmt.Errorf("Non-zero exit status. See output above for more information.")
	}
	return nil
}

func (p *Provisioner) uploadDir(ui packersdk.Ui, comm packersdk.Communicator, dst, src string) error {
	if err := p.createDir(ui, comm, dst); err != nil {
		return err
	}

	// Make sure there is a trailing "/" so that the directory isn't
	// created on the other side.
	if src[len(src)-1] != '/' {
		src = src + "/"
	}
	return comm.UploadDir(dst, src, nil)
}

func (p *Provisioner) getMapValue(ui packersdk.Ui, valueName string) string {

	if p.config.IsWindows {
		valueName = valueName + "W"
	} else {
		valueName = valueName + "L"
	}
	ui.Message(fmt.Sprintf("valueName: %s", valueName))

	value := saltProvisionerMap[valueName]
	ui.Message(fmt.Sprintf("value: %s", value))

	if p.config.IsWindows && value[0:2] == "PS:" {
		value = value[4 : len(value)-1]
		template := saltProvisionerMap["cmdTemplateW"]
		value = fmt.Sprintf(template, value)
	}

	if p.config.UseSudo && valueName[0:2] == "cmd" {
		value = fmt.Sprintf("sudo %s", value)
	}

	ui.Message(fmt.Sprintf("value: %s", value))
	return value
}
