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

var saltConfigMap = map[string]string{
	"configStagingDir_linux":   "/tmp/packer-provisioner-salt",
	"configStagingDir_windows": "C:/Windows/Temp/packer-provisioner-salt",
}

var saltCommandMap = map[string]string{
	"cmdCreateDir_linux":   "mkdir -p '%s'",
	"cmdCreateDir_windows": "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -Command {New-Item -ItemType Directory -Path %s -Force}",
	"cmdDeleteDir_linux":   "rm -rf '%s'",
	"cmdDeleteDir_windows": "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -Command {Remove-Item -Recurse -Force %s}",
	"cmdSaltCall_linux":    "sudo %ssalt-call --local --file-root=%s state.apply %s",
	"cmdSaltCall_windows":  "%ssalt-call --local --file-root=%s state.apply %s",
}

var osTypeMap = map[string]string{
	"amazon":  "linux",
	"arch":    "linux",
	"centos":  "linux",
	"debian":  "linux",
	"fedora":  "linux",
	"freebsd": "linux",
	"linux":   "linux",
	"macos":   "linux",
	"oracle":  "linux",
	"photon":  "linux",
	"redhat":  "linux",
	"suse":    "linux",
	"ubuntu":  "linux",
	"windows": "windows",
}

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	ctx                 interpolate.Context
	// The target OS that the workload is using. This value is used to determine whether a
	// Windows or Linux OS is in use. If not specified, this value defaults to `linux`.
	// In a future version this option may facilitate the installation of the salt-minion.
	TargetOS string `mapstructure:"target_os"`
	// The state files to be applied by Salt. These files must exist on
	// your local system where Packer is executing.
	StateFiles []string `mapstructure:"state_files"`
	// The directory where files will be uploaded. Packer requires write
	// permissions in this directory. Default values are used if this option is no set.
	StagingDir string `mapstructure:"staging_directory"`
	// If set to `true`, the content of the `staging_directory` will be removed after
	// applying Salt states. By default this is set to `false`.
	CleanStagingDir bool `mapstructure:"clean_staging_directory"`
	// Environment variables to make available for Salt.
	// These arguments _will not_ be passed through a shell and arguments should
	// not be quoted. Usage example:
	//
	// ```json
	//    "extra_arguments": [ "--extra-vars", "Region={{user `Region`}} Stage={{user `Stage`}}" ]
	// ```
	//
	// In certain scenarios where you want to pass ansible command line arguments
	// that include parameter and value (for example `--vault-password-file pwfile`),
	// from ansible documentation this is correct format but that is NOT accepted here.
	// Instead you need to do it like `--vault-password-file=pwfile`.
	//
	// If you are running a Windows build on AWS, Azure, Google Compute, or OpenStack
	// and would like to access the auto-generated password that Packer uses to
	// connect to a Windows instance via WinRM, you can use the template variable
	//
	// ```build.Password``` in HCL templates or ```{{ build `Password`}}``` in
	// legacy JSON templates. For example:
	//
	// in JSON templates:
	//
	// ```json
	// "extra_arguments": [
	//    "--extra-vars", "winrm_password={{ build `Password`}}"
	// ]
	// ```
	//
	// in HCL templates:
	// ```hcl
	// extra_arguments = [
	//    "--extra-vars", "winrm_password=${build.Password}"
	// ]
	// ```
	EnvVars []string `mapstructure:"env_vars"`
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
	// Ensure that the target OS is set.
	if p.config.TargetOS == "" {
		p.config.TargetOS = "linux"
	} else {
		p.config.TargetOS = strings.ToLower(p.config.TargetOS)
	}
	// Configure the staging directory
	if p.config.StagingDir == "" {
		p.config.StagingDir = p.getConfig("configStagingDir")
	}

	// Validation
	var errs *packersdk.MultiError
	// Check that state_files is specified
	if len(p.config.StateFiles) == 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("The parameter state_files must be specified"))
	}
	// Check that the files in state_files exist
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

	if len(p.config.PlaybookPaths) > 0 {
		ui.Message("Uploading additional Playbooks...")
		playbookDir := filepath.ToSlash(filepath.Join(p.config.StagingDir, "playbooks"))
		if err := p.createDir(ui, comm, playbookDir); err != nil {
			return fmt.Errorf("Error creating playbooks directory: %s", err)
		}
		for _, src := range p.config.PlaybookPaths {
			dst := filepath.ToSlash(filepath.Join(playbookDir, filepath.Base(src)))
			if err := p.uploadDir(ui, comm, dst, src); err != nil {
				return fmt.Errorf("Error uploading playbooks: %s", err)
			}
		}
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
	// Prepare environment variables
	envVars := ""
	if len(p.config.EnvVars) > 0 {
		envVars = envVars + strings.Join(p.config.EnvVars, " ") + " "
	}

	// Execute Salt
	for _, stateFile := range p.stateFiles {
		if err := p.executeSaltState(ui, comm, envVars, stateFile); err != nil {
			return err
		}
	}

	return nil
}

func (p *Provisioner) executeSaltState(
	ui packersdk.Ui, comm packersdk.Communicator, envVars string, stateFile string,
) error {
	ctx := context.TODO()
	stateName := strings.ReplaceAll(stateFile, ".sls", "")
	command := p.getCommand("cmdSaltCall")
	command = fmt.Sprintf(command, envVars, p.config.StagingDir, stateName)
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
	command := p.getCommand("cmdCreateDir")
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
	command := p.getCommand("cmdDeleteDir")
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

func (p *Provisioner) getCommand(valueName string) string {

	valueName = valueName + "_" + p.getOSType()
	return saltCommandMap[valueName]
}

func (p *Provisioner) getConfig(valueName string) string {

	valueName = valueName + "_" + p.getOSType()
	return saltConfigMap[valueName]
}

func (p *Provisioner) getOSType() string {

	return osTypeMap[p.config.TargetOS]
}
