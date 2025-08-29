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
	// The target OS that the workload is using. This value is used to determine whether a
	// Windows or Linux OS is in use. If not specified, this value defaults to `linux`.
	// Supported values for the selection are:
	//
	// `linux` - This denotes that the target runs a Linux or Unix operating system.
	// `windows` - This denotes that the target runs a Windows operating system.
	//
	// Presently this option determines some of the defaults used by the provisioner.
	TargetOS string `mapstructure:"target_os"`

	// The individual state files to be applied by Salt. These files must exist on
	// your local system where Packer is executing. State files are applied in the order
	// in which they appear in the parameter. This option is exclusive
	// with `state_tree`.
	StateFiles []string `mapstructure:"state_files"`

	// A path to the complete Salt state tree on your local system to be copied to the remote machine as the
	// `state_directory`. The structure of the state tree is flexible, however the use of this option assumes
	// that a `top.sls` file is present at the top of the state tree. The plugin assumes that Salt will evaluate
	// the `top.sls` file and match expressions to determine which individual states should be applied. This action
	// is referred to as a "highstate". This option is exclusive with `state_files`.
	//
	// For more details about states and highstates, refer to the [Salt documentation](https://docs.saltproject.io/en/latest/topics/tutorials/starting_states.html).
	StateTree string `mapstructure:"state_tree"`

	// Directory where files will be uploaded to on the target system.
	// NOTE: Deprecated. Use state_directory instead.
	StagingDir string `mapstructure:"staging_directory"`

	// The directory where state files will be uploaded to on the target system. Packer requires write
	// permissions in this directory. Default values are used if this option is not set.
	// The default value used will depend on the value of `target_os`. The default for Linux systems is:
	//
	// ```
	// /tmp/packer-provisioner-salt
	// ```
	//
	// For Windows systems the default is:
	//
	// ```
	// C:/Windows/Temp/packer-provisioner-salt
	// ```
	//
	// Windows paths are recommended to be set using `/` as the delimiter owing to more conventional
	// characters causing issues when this plugin is executed on a Linux system.
	StateDir string `mapstructure:"state_directory"`

	// The individual pillar files to be used by Salt. These files must exist on
	// the local system where Packer is executing. Individual pillar files must be referenced
	// directly by state files unless a 'top.sls' file is included. This option is exclusive
	// with `pillar_tree`.
	PillarFiles []string `mapstructure:"pillar_files"`

	// A path to the complete Salt pillar tree on your local system to be copied to the remote machine as the
	// `pillar_directory`. The structure of the pillar tree is flexible, however the use of this option assumes
	// that a `top.sls` file is present at the top of the pillar tree. The plugin assumes that Salt will evaluate
	// the `top.sls` file and match expressions to determine which individual pillars should be applied.
	// This option is exclusive with `pillar_files`.
	//
	// For more details about pillars, refer to the [Salt documentation](https://docs.saltproject.io/salt/user-guide/en/latest/topics/pillar.html).
	PillarTree string `mapstructure:"pillar_tree"`

	// The directory where pillar files will be uploaded to on the target system. Packer requires write
	// permissions in this directory. Default values are used if this option is not set.
	// The default value used will depend on the value of `target_os`. The default for Linux systems is:
	//
	// ```
	// /tmp/packer-provisioner-salt-pillar
	// ```
	//
	// For Windows systems the default is:
	//
	// ```
	// C:/Windows/Temp/packer-provisioner-salt-pillar
	// ```
	//
	// Windows paths are recommended to be set using `/` as the delimiter owing to more conventional
	// characters causing issues when this plugin is executed on a Linux system.
	PillarDir string `mapstructure:"pillar_directory"`

	// If set to `true`, the contents uploaded to the target system will be removed after
	// applying Salt states. By default this is set to `false`.
	Clean bool `mapstructure:"clean"`

	// A collection of environment variables that will be made available to the Salt process
	// when it is executed. The intended purpose of this facility is to enable secrets or
	// environment-specific information to be consumed when applying Salt states.
	//
	// For example:
	//
	// ```hcl
	// environment_vars = [ "SECRET_VALUE=${ var.build_secret }",
	//                      "CONFIG_VALUE=${ var.config_value }" ]
	// ```
	// This would expose the environment variables `SECRET_VALUE` and `CONFIG_VALUE` to the Salt process.
	// These environment variables can then be consumed within Salt states, for example:
	//
	// ```text
	// {% set secret_value = salt['environ.get']('SECRET_VALUE', 'default_value') %}
	// {% set config_value = salt['environ.get']('CONFIG_VALUE', 'default_value') %}
	// # Echo config value
	// echo config value:
	// cmd.run:
	//  - name: echo {{ config_value }}
	// ```
	EnvVars []string `mapstructure:"environment_vars"`

	// Format string for environment variables. Default: "VARNAME='VARVALUE' ".
	// NOTE: Deprecated.
	EnvVarFormat string `mapstructure:"env_var_format"`
}

type Provisioner struct {
	config        Config
	stateFiles    []string
	pillarFiles   []string
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

	var errs *packersdk.MultiError

	// Set default values
	if p.config.TargetOS == "" {
		p.config.TargetOS = "linux"
	} else {
		p.config.TargetOS = strings.ToLower(p.config.TargetOS)
	}
	if p.config.EnvVars == nil {
		p.config.EnvVars = []string{}
	}
	if p.config.StateFiles == nil {
		p.config.StateFiles = []string{}
	}
	if p.config.PillarFiles == nil {
		p.config.PillarFiles = []string{}
	}
	if p.config.EnvVarFormat == "" {
		p.config.EnvVarFormat = p.getConfig("configEnvFormat")
	}
	if p.config.StateDir == "" {
		if p.config.StagingDir != "" {
			p.config.StateDir = p.config.StagingDir
		} else {
			p.config.StateDir = p.getConfig("configStateDir")
		}
	}
	if p.config.PillarDir == "" {
		p.config.PillarDir = p.getConfig("configPillarDir")
	}

	// Validate exclusive options
	if len(p.config.StateFiles) != 0 && p.config.StateTree != "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("either state_files or state_tree can be specified, not both"))
	}
	if len(p.config.StateFiles) == 0 && p.config.StateTree == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("either state_files or state_tree must be specified"))
	}
	if len(p.config.PillarFiles) != 0 && p.config.PillarTree != "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("either pillar_files or pillar_tree can be specified, not both"))
	}

	// Validate any supplied environment variables
	for _, kv := range p.config.EnvVars {
		vs := strings.SplitN(kv, "=", 2)
		if len(vs) != 2 || vs[0] == "" {
			errs = packersdk.MultiErrorAppend(errs,
				fmt.Errorf("environment variable not in format 'key=value': %s", kv))
		}
	}

	// Validate supplied arrays of files
	for _, f := range p.config.StateFiles {
		if err := validateFileConfig(f, "state_files"); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		} else {
			p.stateFiles = append(p.stateFiles, f)
		}
	}
	for _, f := range p.config.PillarFiles {
		if err := validateFileConfig(f, "pillar_files"); err != nil {
			errs = packersdk.MultiErrorAppend(errs, err)
		} else {
			p.pillarFiles = append(p.pillarFiles, f)
		}
	}

	// Vaildate supplied file trees
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

	// Upload state tree or create directory for state files
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

	// Upload pillar tree
	if p.config.PillarTree != "" {
		ui.Say("Uploading Pillar Tree...")
		if err := p.uploadDir(ui, comm, p.config.PillarDir, p.config.PillarTree); err != nil {
			return fmt.Errorf("error uploading pillar_tree: %s", err)
		}
	}

	// Create directory for pillar files
	if len(p.pillarFiles) > 0 {
		ui.Say("Creating Salt pillar directory...")
		if err := p.createDir(ui, comm, p.config.PillarDir); err != nil {
			return fmt.Errorf("error creating pillar directory: %s", err)
		}
	}

	// Upload state files
	if len(p.stateFiles) > 0 {
		if err := p.uploadFiles(ui, comm, p.stateFiles, p.config.StateDir); err != nil {
			return err
		}
	}

	// Upload pillar files
	if len(p.pillarFiles) > 0 {
		if err := p.uploadFiles(ui, comm, p.pillarFiles, p.config.PillarDir); err != nil {
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
func (p *Provisioner) uploadFiles(ui packersdk.Ui, comm packersdk.Communicator, sourceFiles []string, targetDir string) error {
	for _, f := range sourceFiles {
		if err := p.uploadSingleFile(ui, comm, f, targetDir); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provisioner) uploadSingleFile(ui packersdk.Ui, comm packersdk.Communicator, uploadFile string, uploadDir string) error {
	localFile, _ := filepath.Abs(uploadFile)
	ui.Say(fmt.Sprintf("Uploading file %s to %s", localFile, uploadDir))

	remoteDir := filepath.ToSlash(filepath.Join(uploadDir, filepath.Dir(uploadFile)))
	remoteFile := filepath.ToSlash(filepath.Join(uploadDir, uploadFile))

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
	ui.Say(fmt.Sprintf("Uploading local directory %s to remote directory %s", src, dst))
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
