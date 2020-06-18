Licence management scripts
==========================

This directory contains the scripts to generate licence notice and dependency information documentation.

- `generate-notice.sh`: Invoked by `make generate-notice-file` to automatically generate `NOTICE.txt` and `docs/dependencies.asciidoc`.
- `generate-image-deps.sh`: Manually invoked script to update the container image dependency information in `docs/container-image-dependencies.csv`.


generate-notice.sh
-------------------

This script invokes `go-licence-detector` application which parses the output of `go list -m -json all` to produce the notice and dependency list. See the documentation at https://github.com/elastic/go-licence-detector for usage information.

### Adding Overrides

In some cases, the licence-detector will not be able to detect the licence type or infer the correct URL for a dependency. When there are issues with licences (no licence file or unknown licence type), the application will fail with an error message instructing the user to add an override to continue. The format of the overrides file is documented in the README file of the tool.

Current overrides file can be found in the `overrides` directory. Follow the existing directory layout (`licences/<domain>/<pkg>/LICENCE`) when adding new licence text overrides.

### Allowing licence types

Current list of allowed licence types can be found in the `rules.json` file. Refer to the documentation in the tool repo for more information about adding new licence types to the allowlist.


generate-image-deps.sh
-----------------------

This script generates licence information for the contents of the ECK container base image. As the container base image is rarely ever changed and the tool used ([Tern](https://github.com/vmware/tern)) is slow to run, this script is not invoked automatically by the build process.

To generate the dependency list (`docs/container-image-dependencies.csv`) for a particular image tag, invoke the script as follows:

```shell
IMAGE_TAG=1.0.1 ./generate-image-deps.sh
```

Note that Tern is Linux only (there are Vagrant instructions for OSX in the Tern repo) and requires sudo access to mount the procfs file system and inspect the container layers. The script will prompt for the sudo password when needed.

This script requires Docker, Python, and jq to be installed on the machine.
