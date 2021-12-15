# Redhat Release Operations Commands

Set of redhat commands to simplify operations required when releasing new versions of the operator for openshift, and the redhat operator registry. 

These commands include

- Publishing eck operator container image to redhat catalog
- Generate Operator Lifecycle Manager format files
- Generate Operator bundle metadata (wrapper for [OPM](https://github.com/operator-framework/operator-registry/tree/master/cmd/opm) command)
- Publish pull request to https://github.com/redhat-openshift-ecosystem/certified-operators

## Overview

```shell
Manage redhat release operations, such as pushing operator container to redhat catalog, operator hub release generation, building operator metadata,
and potentially creating pull request to github.com/redhat-openshift-ecosystem/certified-operators repository.

Usage:
  redhat [command]

Available Commands:
  all         run all redhat operations
  bundle      generate operator bundle metadata, and potentially create pull request for new operator version
  completion  Generate the autocompletion script for the specified shell
  container   push and publish eck operator container to redhat catalog
  help        Help about any command
  operatorhub Generate Operator Lifecycle Manager format files

Flags:
  -h, --help         help for redhat
  -t, --tag string   tag/new version of operator (TAG)
  -v, --version      version for redhat

Use "redhat [command] --help" for more information about a command.
```

## Commands

### Container

```shell
Push and/or Publish eck operator container image to redhat catalog

Usage:
  redhat container [command]

Available Commands:
  publish     publish existing eck operator container image within redhat catalog
  push        push eck operator container image to redhat catalog

Flags:
  -a, --api-key string                       api key to use when communicating with redhat catalog api (API_KEY)
      --enable-vault                         Enable vault functionality to try and automatically read 'redhat-connect-registry-key', and 'api-key' from given vault key (uses VAULT_* environment variables) (ENABLE_VAULT)
  -F, --force                                force will force the attempted pushing of remote images, even when the exact version is found remotely. (FORCE)
  -h, --help                                 help for container
  -p, --project-id string                    short project id within the redhat technology portal (PROJECT_ID) (default "5fa1f9fc4bbec60adbc8cc94")
  -r, --redhat-connect-registry-key string   registry key used to communicate with redhat docker registry (REDHAT_CONNECT_REGISTRY_KEY)
  -R, --repository-id string                 repository project id (ospid) within the redhat technology portal (REPOSITORY_ID) (default "ospid-664938b1-f0c8-4989-99de-be0992395aa0")
      --vault-addr string                    Vault address to use when enable-vault is set
      --vault-secret string                  When --enable-vault is set, attempts to read 'redhat-connect-registry-key', and 'api-key' data from given vault secret location
      --vault-token string                   Vault token to use when enable-vault is set

Global Flags:
  -t, --tag string   tag/new version of operator (TAG)

Use "redhat container [command] --help" for more information about a command.
```

The `container push` sub-command will perform the following tasks:

1. Determine if there is an image in the [redhat container catalog api](https://catalog.redhat.com/api/containers/v1) that has the given `tag`, using the provided `project-id`.
2. If image is already found, nothing is done without using the `force` flag.
3. If image not found, or `force` flag set, will push `docker.elastic.co/eck/eck-operator:$(tag)` to `scan.connect.redhat.com` docker registry, tagged as `scan.connect.redhat.com/$(repository-id)/eck-operator:$(tag)`.

The `container publish` sub-command will perform the following tasks:
1. It will wait for the image to be found in the container catalog api.
2. It will wait for the image scan to be successful in the container catalog api.
3. It will "publish" the container.

#### Usage

*notes*
- `api-key` is the redhat api key that exists within the keybase application
- `redhat-connect-registry-key` is the JWT that can be obtained by logging into [redhat connect](https://connect.redhat.com) and clicking [Push Image Manually](https://connect.redhat.com/projects/5fa1f9fc4bbec60adbc8cc94/images/upload-image), which will then show you a `Registry Key`
- both of these can be contained within a secret in vault, and be pulled directly from vault

Usage without vault
```shell
./redhat container publish -a 'api-key-in-keybase' -d -r 'extremely-long-registry-key-jwt' -t 1.9.2
```

Usage with vault
```shell
VAULT_ADDR=http://vault-server VAULT_TOKEN=my-token ./redhat container publish --enable-vault --vault-secret /secret/data/release/redhat-registry/eck-team -t 1.9.2
```

Example vault secret
```shell
❯ VAULT_ADDR=http://localhost:8200 VAULT_TOKEN=my-token vault kv get secret/release/redhat-registry/eck-team
======= Metadata =======
Key                Value
---                -----
created_time       2022-01-05T19:25:24.014404651Z
custom_metadata    <nil>
deletion_time      n/a
destroyed          false
version            3

=============== Data ===============
Key                            Value
---                            -----
api-key                        random-api-key
redhat-connect-registry-key    very.long.jwt
```

### Bundle

```shell
Bundle and build operator metadata for publishing on openshift operator hub, and potentially create pull request to
github.com/redhat-openshift-ecosystem/certified-operators repository.

Usage:
  redhat bundle [command]

Available Commands:
  create-pr   create pull request against github.com/redhat-openshift-ecosystem/certified-operators repository
  generate    generate operator bundle metadata

Flags:
  -h, --help   help for bundle

Global Flags:
  -t, --tag string   tag/new version of operator (TAG)

Use "redhat bundle [command] --help" for more information about a command.
```

*This command requires the output of the `operatorhub` command to be run*

The bundle command will perform the following tasks:

1. Run the [opm](https://github.com/operator-framework/operator-registry/tree/master/cmd/opm) command on the output of `operatorhub` command to generate the operator metadata for operator hub publishing
2. If the pull request option is set `-P`, will create a pull request at https://github.com/redhat-openshift-ecosystem/certified-operators repository using the output of both `operatorhub` command, and the `bundle` command.

#### Usage

- Ensure that the `operatorhub` command has successfully ran
- Run the bundle command, pointing to the output directory of the `operatorhub` command

Generate operator metadata
```shell
./redhat bundle generate -d ./certified-operators -o "v4.6-v4.9"
```

Create pull request
```shell
./redhat bundle create-pr -d ./certified-operators -f 'Your Name' -g 'your-github-token' -u 'your-github-username' -e 'your-github-email' -t 'release-tag'
```

### Operatorhub

```shell
Generate Operator Lifecycle Manager format files

Usage:
  redhat operatorhub [flags]

Examples:
redhat operatorhub -t 1.9.2 -V 1.9.1 -s 7.16.0

Flags:
  -c, --conf string             Path to config file to read CRDs, and packages (CONF) (default "./config.yaml")
  -h, --help                    help for operatorhub
  -V, --prev-version string     Previous version of the operator to populate 'replaces' in operator cluster service version yaml (PREV_VERSION)
  -s, --stack-version string    Stack version of Elastic stack used to populate the operator cluster service version yaml (STACK_VERSION)
  -T, --templates string        Path to the templates directory (TEMPLATES) (default "./templates")
  -y, --yaml-manifest strings   Path to installation manifests (YAML_MANIFEST)

Global Flags:
  -t, --tag string   tag/new version of operator (TAG)
```

Extracts CRDs and RBAC definitions from distribution YAML manifests and generates the files required to publish a new release to Operator Hub.

#### Usage

- Edit `config.yaml` and update the values to match the new release
- Run the generator
- Inspect the generated files to make sure that they are correct

```shell
go run main.go operatorhub -c config.yaml -t 1.9.2
```

To generate configuration based on yet unreleased YAML manifests:

```shell
go run main.go operatorhub -c config.yaml -y ../../config/crds.yaml -y ./../config/operator.yaml -s '7.16.0' -V '1.9.1' -t 1.9.2
```

IMPORTANT: The operator deployment spec is different from the spec in `operator.yaml` and cannot be automatically extracted from it. Therefore, the deployment spec is hardcoded into the template and should be checked with each new release to ensure that it is still correct.

### All

```shell
Run all redhat operations: push operator container; create operatorhub manifests; create operator bundle images and create PR to redhat certified operators repository

Usage:
  redhat all [flags]

Flags:
  -a, --api-key string                       api key to use when communicating with redhat catalog api (API_KEY)
  -c, --conf string                          Path to config file to read CRDs, and packages (CONF) (default "./config.yaml")
  -D, --dry-run                              dry-run will only process images locally, and will not make any changes within redhat connect
  -F, --force                                force will force the attempted pushing of remote images, even when the exact version is found remotely.
  -e, --github-email string                  if 'submit-pull-request' is enabled, github email to use to add to commit message (GITHUB_EMAIL)
  -f, --github-fullname string               if 'submit-pull-request' is enabled, github full name to use to add to commit message (GITHUB_FULLNAME)
  -g, --github-token string                  if 'submit-pull-request' is enabled, user's token to communicate with github.com (GITHUB_TOKEN)
  -u, --github-username string               if 'submit-pull-request' is enabled, github username to use to fork repo, and submit PR
  -h, --help                                 help for all
  -K, --keep-temp-files                      keep temporary files around for investigation after script completes (KEEP_TEMP_FILES)
  -V, --prev-version string                  Previous version of the operator to populate 'replaces' in operator cluster service version yaml (PREV_VERSION)
  -p, --project-id string                    short project id within the redhat technology portal (PROJECT_ID) (default "5fa1f9fc4bbec60adbc8cc94")
  -r, --redhat-connect-registry-key string   registry key used to communicate with redhat docker registry (REDHAT_CONNECT_REGISTRY_KEY)
  -R, --repository-id string                 repository project id (ospid) within the redhat technology portal (REPOSITORY_ID) (default "ospid-664938b1-f0c8-4989-99de-be0992395aa0")
  -s, --stack-version string                 Stack version of Elastic stack used to populate the operator cluster service version yaml (STACK_VERSION)
  -P, --submit-pull-request                  attempt to submit PR to https://github.com/redhat-openshift-ecosystem/certified-operators repo? (SUBMIT_PULL_REQUEST)
  -t, --tag string                           tag/new version of operator (TAG)
  -T, --templates string                     Path to the templates directory (TEMPLATES) (default "./templates")
  -y, --yaml-manifest strings                Path to installation manifests (YAML_MANIFEST)
```

The `all` command will run all above operations in a single command, including

- `container` command to push the container image
- `operatorhub` command to generate Operator Lifecycle Manager format files
- `bundle` command to generate manifest bundle, and potentially create pull request

#### Usage

```shell
./redhat all -a 'api-key-in-keybase' -r 'extremely-long-registry-key-jwt' \
  -f 'Your Name' -g 'your-github-token' -u 'your-github-username' -e 'your-github-email' \
  -t '1.9.2' -s '7.16.0' -V '1.9.1' -y ../../config/crds.yaml -y ./../config/operator.yaml
```

