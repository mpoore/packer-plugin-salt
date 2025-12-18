The `Salt` plugin enables users to customize image builds using [Salt](https://saltproject.io) states by offering a provisioner dedicated to that purpose.

### Installation

To install this plugin, copy and paste this code into your Packer configuration, then run [`packer init`](https://www.packer.io/docs/commands/init).

```hcl
packer {
  required_plugins {
    salt = {
      # source represents the GitHub URI to the plugin repository without the `packer-plugin-` prefix.
      source  = "github.com/mpoore/salt"
      version = ">=0.5.6"
    }
  }
}
```

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
$ packer plugins install github.com/mpoore/salt
```

### Components

**Note:** The current version of this plugin does **not** automatically install the required `salt-minion` package. It is assumed when calling this provisioner that installation of the Salt Minion has already taken place. Commonly users may employ the shell provisioner (or similar) to install the Salt Minion or include the necessary steps within their KickStart or seed file for their build. Instructions for installing the Salt Minion are be located on the [SaltProject website](https://docs.saltproject.io/salt/install-guide/en/latest/).

**Note:** The Salt Minion package need only be installed, it does not need to be enabled as a service or configured with a Salt Master.

#### Provisioners

- [salt](https://developer.hashicorp.com/packer/integrations/mpoore/salt/latest/components/provisioner/salt) - The Packer provisioner will transfer Salt state files to the target guest operating system and execute `Salt` to apply the configured desired state.