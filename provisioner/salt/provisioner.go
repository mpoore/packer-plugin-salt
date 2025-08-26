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
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

var saltConfigMap = map[string]string{
	"configStateDir_linux":    "/tmp/packer-provisioner-salt",
	"configStateDir_windows":  "C:/Windows/Temp/packer-provisioner-salt",
	"configPillarDir_linux":   "/tmp/packer-provisioner-salt-pillar",
	"configPillarDir_windows": "C:/Windows/Temp/packer-provisioner-salt-pillar",
	"configEnvFormat_linux":   "%s='%s' ",
	"configEnvFormat_windows": "%s='%s' ",
}

var saltCommandMap = map[string]string{
	"cmdCreateDir_linux":        "mkdir -p '%s'",
	"cmdCreateDir_windows":      "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -Command {New-Item -ItemType Directory -Path %s -Force}",
	"cmdDeleteDir_linux":        "rm -rf '%s'",
	"cmdDeleteDir_windows":      "PowerShell -ExecutionPolicy Bypass -OutputFormat Text -Command {Remove-Item -Recurse -Force %s}",
	"cmdSaltCall_linux":         "sudo %ssalt-call --local --file-root=%s state.apply %s",
	"cmdSaltCall_windows":       "%ssalt-call --local --file-root=%s state.apply %s",
	"cmdSaltCallPillar_linux":   "sudo %ssalt-call --local --file-root=%s --pillar-root=%s state.apply %s",
	"cmdSaltCallPillar_windows": "%ssalt-call --local --file-root=%s --pillar-root=%s state.apply %s",
}

type Config struct {
	// Embedded Packer fields (e.g. Communicator, PauseBefore, etc.)
	common.PackerConfig `mapstructure:",squash"`
	// Internal interpolation context, not user-configurable.
	ctx interpolate.Context

	// USER FACING CONFIGURATION
	// The target OS that the workload is using. Defaults to "linux".
	// Supported values: "linux", "windows".
	TargetOS string `mapstructure:"target_os"`

	// List of individual state files to apply. Exclusive with state_tree.
	StateFiles []string `mapstructure:"state_files"`

	// Path to the complete Salt State Tree. Exclusive with state_files.
	StateTree string `mapstructure:"state_tree"`

	// Directory where files will be uploaded to on the target system.
	// NOTE: Deprecated. Use StateDir instead.
	StagingDir string `mapstructure:"staging_directory"`

	// Directory where files will be uploaded to on the target system.
	StateDir string `mapstructure:"state_directory"`

	// Path to Salt Pillar data tree to be copied to the remote machine.
	PillarTree string `mapstructure:"pillar_tree"`

	// Directory where pillar data files will be uploaded to on the target system.
	PillarDir string `mapstructure:"pillar_directory"`

	// If true, remove uploaded contents after applying states. Defaults to false.
	Clean bool `mapstructure:"clean"`

	// Environment variables to be set for the Salt process.
	EnvVars []string `mapstructure:"environment_vars"`

	// Format string for environment variables. Default: "VARNAME='VARVALUE' ".
	EnvVarFormat string `mapstructure:"env_var_format"`
}

type Provisioner struct {
	config        Config
	stateFiles    []string
	generatedData map[string]interface{}
}

// ----------------------------------------------------------------------------
// Configure and prepare
// ----------------------------------------------------------------------------
func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

func (p *Provisioner) Prepare(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         "salt",
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
	}, raws...)
	if err != nil {
		return err
	}

	if p.config.TargetOS == "" {
		p.config.TargetOS = "linux"
	}
	if p.config.EnvVars == nil {
		p.config.EnvVars = []string{}
	}
	if p.config.StateFiles == nil {
		p.config.StateFiles = []string{}
	}

	p.stateFiles = make([]string, 0, len(p.config.StateFiles))
	var errs *packersdk.MultiError

	if len(p.config.StateFiles) != 0 && p.config.StateTree != "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("either state_files or state_tree can be specified, not both"))
	}
	if len(p.config.StateFiles) == 0 && p.config.StateTree == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("either state_files or state_tree must be specified"))
	}

	for _, f := range p.config.StateFiles {
		if err := validateFileConfig(f, "state_files"); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		} else {
			p.stateFiles = append(p.stateFiles, f)
		}
	}

	if p.config.StateTree != "" {
		if err := validateDirConfig(p.config.StateTree, "state_tree"); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}
	if p.config.PillarTree != "" {
		if err := validateDirConfig(p.config.PillarTree, "pillar_tree"); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		}
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

// ----------------------------------------------------------------------------
// Provision method
// ----------------------------------------------------------------------------
func (p *Provisioner) Provision(ctx context.Context, ui packersdk.Ui, comm packersdk.Communicator, generatedData map[string]interface{}) error {
	p.generatedData = generatedData
	ui.Say("Provisioning with Salt...")

	if p.config.StateTree != "" {
		ui.Say("Uploading State Tree...")
		if err := p.uploadDir(ui, comm, p.config.StateDir, p.config.StateTree); err != nil {
			return fmt.Errorf("error uploading state_tree: %s", err)
		}
	} else {
		ui.Say("Creating Salt state directory...")
		if err := p.createDir(ui, comm, p.config.StateDir); err != nil {
			return fmt.Errorf("error creating state directory: %s", err)
		}
	}

	if p.config.PillarTree != "" {
		ui.Say("Uploading Pillar Tree...")
		if err := p.uploadDir(ui, comm, p.config.PillarDir, p.config.PillarTree); err != nil {
			return fmt.Errorf("error uploading pillar_tree: %s", err)
		}
	}

	if len(p.stateFiles) > 0 {
		if err := p.uploadStateFiles(ui, comm); err != nil {
			return err
		}
	}

	if err := p.executeSalt(ui, comm); err != nil {
		return fmt.Errorf("error executing Salt: %s", err)
	}

	if p.config.Clean {
		ui.Say("Cleaning up state and pillar directories...")
		_ = p.removeDir(ui, comm, p.config.StateDir)
		_ = p.removeDir(ui, comm, p.config.PillarDir)
	}

	return nil
}

// ----------------------------------------------------------------------------
// File and directory helper methods
// ----------------------------------------------------------------------------
func (p *Provisioner) uploadStateFiles(ui packersdk.Ui, comm packersdk.Communicator) error {
	for _, f := range p.stateFiles {
		if err := p.uploadStateFile(ui, comm, f); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) uploadStateFile(ui packersdk.Ui, comm packersdk.Communicator, stateFile string) error {
	localFile, _ := filepath.Abs(stateFile)
	ui.Say(fmt.Sprintf("Uploading state file: %s", localFile))

	remoteDir := filepath.ToSlash(filepath.Join(p.config.StateDir, filepath.Dir(stateFile)))
	remoteFile := filepath.ToSlash(filepath.Join(p.config.StateDir, stateFile))

	if err := p.createDir(ui, comm, remoteDir); err != nil {
		return err
	}

	if err := p.uploadFile(ui, comm, remoteFile, localFile); err != nil {
		return err
	}
	return nil
}

func (p *Provisioner) uploadDir(ui packersdk.Ui, comm packersdk.Communicator, dst, src string) error {
	if err := p.createDir(ui, comm, dst); err != nil {
		return err
	}
	if src[len(src)-1] != '/' {
		src += "/"
	}
	return comm.UploadDir(dst, src, nil)
}

func (p *Provisioner) createDir(ui packersdk.Ui, comm packersdk.Communicator, dir string) error {
	cmd := &packersdk.RemoteCmd{Command: fmt.Sprintf("mkdir -p '%s'", dir)}
	ui.Say(fmt.Sprintf("Creating directory: %s", dir))
	if err := cmd.RunWithUi(context.TODO(), comm, ui); err != nil {
		return err
	}
	if cmd.ExitStatus() != 0 {
		return fmt.Errorf("non-zero exit status while creating directory")
	}
	return nil
}

func (p *Provisioner) removeDir(ui packersdk.Ui, comm packersdk.Communicator, dir string) error {
	cmd := &packersdk.RemoteCmd{Command: fmt.Sprintf("rm -rf '%s'", dir)}
	ui.Say(fmt.Sprintf("Removing directory: %s", dir))
	_ = cmd.RunWithUi(context.TODO(), comm, ui)
	return nil
}

func (p *Provisioner) uploadFile(ui packersdk.Ui, comm packersdk.Communicator, dst, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening file %s: %s", src, err)
	}
	defer f.Close()
	ui.Say(fmt.Sprintf("Uploading file: %s", src))
	return comm.Upload(dst, f, nil)
}

func validateDirConfig(path string, cfg string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %s invalid: %s", cfg, path, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s: %s must be a directory", cfg, path)
	}
	return nil
}

func validateFileConfig(path string, cfg string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %s invalid: %s", cfg, path, err)
	} else if info.IsDir() {
		return fmt.Errorf("%s: %s must be a file", cfg, path)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Salt execution methods
// ----------------------------------------------------------------------------
func (p *Provisioner) executeSalt(ui packersdk.Ui, comm packersdk.Communicator) error {
	// Prepare environment variables
	envVars := p.createFlattenedEnvVars()

	// Execute Salt
	if len(p.config.StateFiles) != 0 {
		for _, stateFile := range p.stateFiles {
			if err := p.executeSaltState(ui, comm, envVars, stateFile); err != nil {
				return err
			}
		}
	} else {
		if err := p.executeSaltState(ui, comm, envVars, ""); err != nil {
			return err
		}
	}

	return nil
}

func (p *Provisioner) executeSaltState(ui packersdk.Ui, comm packersdk.Communicator, envVars string, stateFile string) error {
	ctx := context.TODO()
	stateName := strings.ReplaceAll(stateFile, ".sls", "")

	var rawCommand string
	if len(p.config.PillarTree) > 0 {
		rawCommand = p.getCommand("cmdSaltCallPillar")
	} else {
		rawCommand = p.getCommand("cmdSaltCall")
	}

	// Select args based on whether PillarTree is present
	var args []any
	if len(p.config.PillarTree) > 0 {
		args = []any{envVars, p.config.StateDir, p.config.PillarDir, stateName}
	} else {
		args = []any{envVars, p.config.StateDir, stateName}
	}

	command := fmt.Sprintf(rawCommand, args...)

	ui.Say(fmt.Sprintf("Executing Salt: %s", command))
	cmd := &packersdk.RemoteCmd{Command: command}

	if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
		return err
	}
	if cmd.ExitStatus() != 0 {
		if cmd.ExitStatus() == 127 {
			return fmt.Errorf("%s could not be found, verify that it is available on the path after connecting to the machine", command)
		}
		return fmt.Errorf("non-zero exit status: %d", cmd.ExitStatus())
	}

	return nil
}

// ----------------------------------------------------------------------------
// Salt execution / configuration helper methods
// ----------------------------------------------------------------------------
func (p *Provisioner) getCommand(valueName string) string {

	valueName = valueName + "_" + p.config.TargetOS
	return saltCommandMap[valueName]
}

func (p *Provisioner) getConfig(valueName string) string {

	valueName = valueName + "_" + p.config.TargetOS
	return saltConfigMap[valueName]
}

func (p *Provisioner) createFlattenedEnvVars() string {
	keys, envVars := p.escapeEnvVars()

	// Re-assemble vars into specified format and flatten
	var flattened string
	for _, key := range keys {
		flattened += fmt.Sprintf(p.config.EnvVarFormat, key, envVars[key])
	}

	return flattened
}

func (p *Provisioner) escapeEnvVars() ([]string, map[string]string) {
	envVars := make(map[string]string)

	// Split vars into key/value components
	for _, envVar := range p.config.EnvVars {
		keyValue := strings.SplitN(envVar, "=", 2)
		// Store pair, replacing any single quotes in value so they parse
		// correctly with required environment variable format
		envVars[keyValue[0]] = strings.Replace(keyValue[1], "'", `'"'"'`, -1)
	}

	// Create a list of env var keys in sorted order
	var keys []string
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys, envVars
}
