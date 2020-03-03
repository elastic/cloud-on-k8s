Licence Detector
================

This directory contains the scripts to generate licence notice and dependency information documentation.

- `generate-notice.sh`: Invoked by `make generate-notice-file` to automatically generate `NOTICE.txt` and `docs/dependencies.asciidoc`.
- `generate-image-deps.sh`: Manually invoked script to update the container image dependency information in `docs/container-image-dependencies.csv`.


generate-notice.sh
-------------------

This script invokes the `licence-detector` application which parses the output of `go list -m -json all` to produce the notice and dependency list.

### Usage

```
licence-detector [FLAGS]

Flags:
  -depsOut string
    	Path to output the dependency list (default "dependencies.asciidoc")
  -depsTemplate string
    	Path to the dependency list template file (default "templates/dependencies.asciidoc.tmpl")
  -in string
    	Dependency list (output from go list -m -json all) (default "-")
  -includeIndirect
    	Include indirect dependencies
  -licenceData string
    	Path to the licence database (default "licence.db")
  -noticeOut string
    	Path to output the notice (default "NOTICE.txt")
  -noticeTemplate string
    	Path to the NOTICE template file (default "templates/NOTICE.txt.tmpl")
  -overrides string
    	Path to the file containing override directives
  -validate
    	Validate results (slow)
```

### Adding Overrides

In some cases, the licence-detector will not be able to detect the licence type or infer the correct URL for a dependency. When there are issues with licences (no licence file or unknown licence type), the application will fail with an error message instructing the user to add an override to continue. The overrides file is a file containing newline-delimited JSON where each line contains a JSON object bearing the following format:

- `name`: Required. Module name to apply the override to.
- `licenceType`: Optional. Type of licence (Apache-2.0, ISC etc.). Provide a [SPDX](https://spdx.org/licenses/) identifier.
- `licenceTextOverrideFile`: Optional. Path to a file containing the licence text for this module. Path must be relative to the `overrides.json` file.
- `url`: Optional. URL to the dependency website.

Example overrides file:

```json
{"name": "github.com/bmizerany/perks", "licenceTextOverrideFile": "licences/github.com/bmizerany/perks/LICENCE"}
{"name": "github.com/dgryski/go-gk", "licenceType": "MIT"}
{"name": "github.com/russross/blackfriday/v2", "url": "https://gopkg.in/russross/blackfriday.v2"}
```

Current overrides file can be found in the `overrides` directory. Follow the existing directory layout (`licences/<domain>/<pkg>/LICENCE`) when adding new licence text overrides.

### Updating the licence database

The licence database file `licence.db` contains all the currently known licence types found in https://github.com/google/licenseclassifier/tree/master/licenses. In the rare case that entirely new licence types have been introduced to the codebase, follow the instructions at https://github.com/google/licenseclassifier to execute the `license_serializer` tool.


generate-image-deps.sh
-----------------------

This script generates licence information for the contents of the ECK container base image. As the container base image is rarely ever changed and the tool used ([Tern](https://github.com/vmware/tern)) is slow to run, this script is not invoked automatically by the build process.

To generate the dependency list (`docs/container-image-dependencies.csv`) for a particular image tag, invoke the script as follows:

```shell
IMAGE_TAG=1.0.1 ./generate-image-deps.sh
```

Note that Tern requires sudo access to mount the procfs file system and inspect the container layers. The script will prompt for the sudo password when needed.

This script requires Docker, Python, and jq to be installed on the machine.
