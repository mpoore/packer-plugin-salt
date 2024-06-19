// Code generated by "packer-sdc mapstructure-to-hcl2"; DO NOT EDIT.

package salt

import (
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"
)

// FlatConfig is an auto-generated flat version of Config.
// Where the contents of a field with a `mapstructure:,squash` tag are bubbled up.
type FlatConfig struct {
	PackerBuildName     *string           `mapstructure:"packer_build_name" cty:"packer_build_name" hcl:"packer_build_name"`
	PackerBuilderType   *string           `mapstructure:"packer_builder_type" cty:"packer_builder_type" hcl:"packer_builder_type"`
	PackerCoreVersion   *string           `mapstructure:"packer_core_version" cty:"packer_core_version" hcl:"packer_core_version"`
	PackerDebug         *bool             `mapstructure:"packer_debug" cty:"packer_debug" hcl:"packer_debug"`
	PackerForce         *bool             `mapstructure:"packer_force" cty:"packer_force" hcl:"packer_force"`
	PackerOnError       *string           `mapstructure:"packer_on_error" cty:"packer_on_error" hcl:"packer_on_error"`
	PackerUserVars      map[string]string `mapstructure:"packer_user_variables" cty:"packer_user_variables" hcl:"packer_user_variables"`
	PackerSensitiveVars []string          `mapstructure:"packer_sensitive_variables" cty:"packer_sensitive_variables" hcl:"packer_sensitive_variables"`
	TargetOS            *string           `mapstructure:"target_os" cty:"target_os" hcl:"target_os"`
	StateFiles          []string          `mapstructure:"state_files" cty:"state_files" hcl:"state_files"`
	StateTree           *string           `mapstructure:"state_tree" cty:"state_tree" hcl:"state_tree"`
	StagingDir          *string           `mapstructure:"staging_directory" cty:"staging_directory" hcl:"staging_directory"`
	Clean               *bool             `mapstructure:"clean" cty:"clean" hcl:"clean"`
	EnvVars             []string          `mapstructure:"environment_vars" cty:"environment_vars" hcl:"environment_vars"`
	EnvVarFormat        *string           `mapstructure:"env_var_format" cty:"env_var_format" hcl:"env_var_format"`
}

// FlatMapstructure returns a new FlatConfig.
// FlatConfig is an auto-generated flat version of Config.
// Where the contents a fields with a `mapstructure:,squash` tag are bubbled up.
func (*Config) FlatMapstructure() interface{ HCL2Spec() map[string]hcldec.Spec } {
	return new(FlatConfig)
}

// HCL2Spec returns the hcl spec of a Config.
// This spec is used by HCL to read the fields of Config.
// The decoded values from this spec will then be applied to a FlatConfig.
func (*FlatConfig) HCL2Spec() map[string]hcldec.Spec {
	s := map[string]hcldec.Spec{
		"packer_build_name":          &hcldec.AttrSpec{Name: "packer_build_name", Type: cty.String, Required: false},
		"packer_builder_type":        &hcldec.AttrSpec{Name: "packer_builder_type", Type: cty.String, Required: false},
		"packer_core_version":        &hcldec.AttrSpec{Name: "packer_core_version", Type: cty.String, Required: false},
		"packer_debug":               &hcldec.AttrSpec{Name: "packer_debug", Type: cty.Bool, Required: false},
		"packer_force":               &hcldec.AttrSpec{Name: "packer_force", Type: cty.Bool, Required: false},
		"packer_on_error":            &hcldec.AttrSpec{Name: "packer_on_error", Type: cty.String, Required: false},
		"packer_user_variables":      &hcldec.AttrSpec{Name: "packer_user_variables", Type: cty.Map(cty.String), Required: false},
		"packer_sensitive_variables": &hcldec.AttrSpec{Name: "packer_sensitive_variables", Type: cty.List(cty.String), Required: false},
		"target_os":                  &hcldec.AttrSpec{Name: "target_os", Type: cty.String, Required: false},
		"state_files":                &hcldec.AttrSpec{Name: "state_files", Type: cty.List(cty.String), Required: false},
		"state_tree":                 &hcldec.AttrSpec{Name: "state_tree", Type: cty.String, Required: false},
		"staging_directory":          &hcldec.AttrSpec{Name: "staging_directory", Type: cty.String, Required: false},
		"clean":                      &hcldec.AttrSpec{Name: "clean", Type: cty.Bool, Required: false},
		"environment_vars":           &hcldec.AttrSpec{Name: "environment_vars", Type: cty.List(cty.String), Required: false},
		"env_var_format":             &hcldec.AttrSpec{Name: "env_var_format", Type: cty.String, Required: false},
	}
	return s
}
