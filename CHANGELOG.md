## 0.5.6 (December 18th, 2025)
### IMPROVEMENTS:
* Added the optional setting of 'log_level', which is used to control console messages from salt-call.

## 0.5.5 (October 29th, 2025)
### IMPROVEMENTS:
* The default OS remains "linux", but the provisioner now detects "windows" automatically.
* The target_os option is to be deprecated in a future release.
* Go version updated to 1.25

# 0.5.1 (October 1st, 2025)
### BUGFIXES:
* Windows environment variables are formatted correctly
* Hardcoded linux command removed from create directory function (naughty developer)
* Hardcoded linux command removed from delete directory function (ooops)

## 0.5.0 (August 29th, 2025)
### IMPROVEMENTS:
* Salt pillar files and file trees are now supported.
* The staging_directory option is to be deprecated in favour of a renamed state_directory option.
* The env_var_format option is to be deprecated.

## 0.1.2 (June 19th, 2024)
### IMPROVEMENTS:
* Documentation completed

## 0.1.1 (June 18, 2024)
* First working release