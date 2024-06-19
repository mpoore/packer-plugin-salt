Type: `salt`

The `ansible-local` Packer provisioner will execute `ansible` in Ansible's "local"
mode on the remote/guest VM using Playbook and Role files that exist on the
guest VM. This means Ansible must be installed on the remote/guest VM.
Playbooks and Roles can be uploaded from your build machine (the one running
Packer) to the vm. Ansible is then run on the guest machine in [local
mode](https://docs.ansible.com/ansible/latest/playbooks_delegation.html#local-playbooks)
via the `ansible-playbook` command.

-> **Note:** The current version of this plugin does **not** automatically install the required `salt-minion` package. It is assumed when calling this provisioner that installation of the `salt-minion` has already taken place. Commonly users may employ the [shell provisioner](/packer/docs/provisioner/shell) (or similar) to install the `salt-minion` or include the necessary steps within their KickStart or seed file for their build. Instructions for installing the `salt-minion` are be located on the [SaltProject website](https://docs.saltproject.io/salt/install-guide/en/latest/).

-> **Note:** The `salt-minion` package need only be installed, it does not need to be enabled as a service or configured with a `salt-master`.

## Basic Example

The example below is fully functional.

**HCL2**

```hcl
variable "topping" {
  type    = string
  default = "mushroom"
}

source "docker" "example" {
  image       = "williamyeh/ansible:ubuntu14.04"
  export_path = "packer_example"
  run_command = ["-d", "-i", "-t", "--entrypoint=/bin/bash", "{{.Image}}"]
}

build {
  sources = [
    "source.docker.example"
  ]

  provisioner "ansible-local" {
    playbook_file   = "./playbook.yml"
    extra_arguments = ["--extra-vars", "\"pizza_toppings=${var.topping}\""]
  }
}
```

where ./playbook.yml contains

```
---
- name: hello world
  hosts: 127.0.0.1
  connection: local

  tasks:
    - command: echo {{ pizza_toppings }}
    - debug: msg="{{ pizza_toppings }}"

```

## Configuration Reference

The reference of available configuration options is listed below.

Required (one, not both, of):

- `state_files` (array of strings) - The individual state files to be applied by Salt. These files must exist on
	your local system where Packer is executing. State files are applied in the order
	in which they appear in the `state_files` parameter.

- `state_tree` (array of strings) - A path to the complete Salt State Tree on your local system to be copied to the remote machine.
  The structure of the State Tree is flexible, however the use of this option assumes
	that a `top.sls` file is present at the top of the State Tree. The plugin assumes that Salt will evaluate
	the `top.sls` file and match expressions to determine which individual states should be applied. This action
	is referred to as a "highstate".

Optional:

<!-- Code generated from the comments of the Config struct in provisioner/salt/provisioner.go; DO NOT EDIT MANUALLY -->

- `target_os` (string) - The target OS that the workload is using. This value is used to determine whether a
  Windows or Linux OS is in use. If not specified, this value defaults to `linux`.
  Supported values for the selection dictated by the supported OS for running `salt-minion`:
  
  amazon, arch, centos, debian, fedora, freebsd, linux, macos, oracle, photon, redhat, suse, ubuntu, windows
  
  Presently this option determines some of the defaults used by the provisioner.

- `state_files` ([]string) - The individual state files to be applied by Salt. These files must exist on
  your local system where Packer is executing. State files are applied in the order
  in which they appear in the parameter. This option is exclusive
  with `state_tree`.

- `state_tree` (string) - A path to the complete Salt State Tree on your local system to be copied to the remote machine as the
  `staging_directory`. The structure of the State Tree is flexible, however the use of this option assumes
  that a `top.sls` file is present at the top of the State Tree. The plugin assumes that Salt will evaluate
  the `top.sls` file and match expressions to determine which individual states should be applied. This action
  is referred to as a "highstate". This option is exclusive with `state_files`.
  
  For more details about states and highstates, refer to the [Salt documentation](https://docs.saltproject.io/en/latest/topics/tutorials/starting_states.html).

- `staging_directory` (string) - The directory where files will be uploaded to on the target system. Packer requires write
  permissions in this directory. Default values are used if this option is not set.
  The default value used will depend on the value of `target_os`. The default for Linux systems is:
  
  ```
  /tmp/packer-provisioner-salt
  ```
  
  For Windows systems the default is:
  
  ```
  C:/Windows/Temp/packer-provisioner-salt
  ```
  
  Windows paths are recommended to be set using `/` as the delimiter owing to more conventional
  characters causing issues when this plugin is executed on a Linux system.

- `clean` (bool) - If set to `true`, the contents uploaded to the target system will be removed after
  applying Salt states. By default this is set to `false`.

- `environment_vars` ([]string) - A collection of environment variables that will be made available to the Salt process
  when it is executed. The intended purpose of this facility is to enable secrets or
  environment-specific information to be consumed when applying Salt states.
  
  For example:
  
  ```hcl
  environment_vars = [ "SECRET_VALUE=${ var.build_secret }",
                       "CONFIG_VALUE=${ var.config_value }" ]
  ```
  This would expose the environment variables `SECRET_VALUE` and `CONFIG_VALUE` to the Salt process.
  These environment variables can then be consumed within Salt states, for example:
  
  ```text
  {% set secret_value = salt['environ.get']('SECRET_VALUE', 'default_value') %}
  {% set config_value = salt['environ.get']('CONFIG_VALUE', 'default_value') %}
  # Echo config value
  echo config value:
  cmd.run:
   - name: echo {{ config_value }}
  ```

- `env_var_format` (string) - An advanced option used to customize the format of the `environment_vars` supplied to the Salt process.
  The default format for environment variables is:
  
  ```
  "VARNAME='VARVALUE' "
  ```
  
  **Note:** There is a trailing space in the default value that is required to separate environment varables from each other.

<!-- End of code generated from the comments of the Config struct in provisioner/salt/provisioner.go; -->


Parameters common to all provisioners:

- `pause_before` (duration) - Sleep for duration before execution.

- `max_retries` (int) - Max times the provisioner will retry in case of failure. Defaults to zero (0). Zero means an error will not be retried.

- `only` (array of string) - Only run the provisioner for listed builder(s)
  by name.

- `override` (object) - Override the builder with different settings for a
  specific builder, eg :

  In HCL2:

  ```hcl
  source "null" "example1" {
    communicator = "none"
  }

  source "null" "example2" {
    communicator = "none"
  }

  build {
    sources = ["source.null.example1", "source.null.example2"]
    provisioner "shell-local" {
      inline = ["echo not overridden"]
      override = {
        example1 = {
          inline = ["echo yes overridden"]
        }
      }
    }
  }
  ```

  In JSON:

  ```json
  {
    "builders": [
      {
        "type": "null",
        "name": "example1",
        "communicator": "none"
      },
      {
        "type": "null",
        "name": "example2",
        "communicator": "none"
      }
    ],
    "provisioners": [
      {
        "type": "shell-local",
        "inline": ["echo not overridden"],
        "override": {
          "example1": {
            "inline": ["echo yes overridden"]
          }
        }
      }
    ]
  }
  ```

- `timeout` (duration) - If the provisioner takes more than for example
  `1h10m1s` or `10m` to finish, the provisioner will timeout and fail.
