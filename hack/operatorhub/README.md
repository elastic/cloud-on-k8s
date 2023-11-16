# OperatorHub Release Operations Commands

Set of commands to simplify operations required when releasing new versions of the operator for openshift/operatorhub, and community/certified operators. 

These commands include

- Pushing operator image to Quay.io
- Publishing eck operator container image within redhat catalog
- Generate Operator Lifecycle Manager registry bundle files
- Publish draft pull requests to both https://github.com/redhat-openshift-ecosystem/certified-operators and https://github.com/k8s-operatorhub/community-operators

## Configuration

All commands require the configuration file `config.yaml` to be modified prior to running the commands.

| Configuration  | Description                                                                       |
|----------------|-----------------------------------------------------------------------------------|
| `newVersion`   | The new version of the ECK Operator that is to be released.                       |
| `prevVersion`  | The previous version of the ECK Operator that `newVersion` will replace.          |
| `stackVersion` | The Elastic Stack version that will be the default for this ECK Operator version. |
| `minSupportedOpenshiftVersion` | Miniumum supported openshift version.                             |

## Commands Overview

* container - parent command
  * push - push operator container to quay.io
  * publish - publish operator container within redhat certification API
* generate-manifests - generate operator lifecycle manager format files
* bundle - parent command
  * create-pr - perform all git operations and create pull requests for community and certified operator repositories

## Vault Details

Certain flags can be automatically read from vault if the `enable-vault` flag is set, typically using the environment variables `VAULT_ADDR` and `VAULT_TOKEN`.  

Example vault secrets
```shell
❯ VAULT_ADDR='http://0.0.0.0:8200' VAULT_TOKEN=myroot vault read -format=json -field=data secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat
{
  "api-key": "api-key-in-keybase",
  "project-id": "project-id",
  "registry-password": "registry-password-for-quay.io"
}
❯ VAULT_ADDR='http://0.0.0.0:8200' VAULT_TOKEN=myroot vault read -format=json -field=data secret/ci/elastic-cloud-on-k8s/operatorhub-release-github
{
  "github-email": "you@email.com",
  "github-fullname": "My Fullname",
  "github-token": "ghp_asdflkj12340987",
  "github-username": "ghusername"
}
```

### Vault Configuration

If `enable-vault` flag is `true` the following keys will attempt to be read from vault:

#### Redhat secrets from `redhat-vault-secret` vault location

`api-key`, `project-id`, and `registry-password`

#### Github secrets from `github-vault-secret` vault location

`github-email`, `github-fullname`, `github-token`, `github-username` 

#### Vault flags

| Parameter              | Description                                                                   | Environment Variable       | Default |
|----------------------  |-------------------------------------------------------------------------------|-------------------------   |---------|
| `--enable-vault`       | If enabled the above variables will attempt to be read from vault.            | `OHUB_ENABLE_VAULT`        | `false` |
| `--vault-addr`         | Vault address to read secrets from.                                           | `VAULT_ADDR`               | `""`    |
| `--vault-token`        | Vault token to use for authentication.                                        | `VAULT_TOKEN`             | `""`    |
| `--github-vault-secret`| Vault secret path to github secrets.                                          | `OHUB_GITHUB_VAULT_SECRET` | `""`    |
| `--redhat-vault-secret`| Vault secret path to redhat secrets.                                          | `OHUB_REDHAT_VAULT_SECRET` | `""`    |


## Container Command

### Push sub-command

The `container push` sub-command will perform the following tasks:
1. Determine if there is an image in the [redhat certification API](https://catalog.redhat.com/api/containers/v1) that has the given `tag` associated with `newVersion`, using the provided `project-id`.
2. If image is already found, nothing is done without using the `force` flag.
3. If image not found, or `force` flag set, will push `docker.elastic.co/eck/eck-operator-ubi:$(newVersion)` to `quay.io` docker registry, tagged as `quay.io/redhat-isv-containers/$(project-id):$(newVersion)`.

### Publish sub-command

The `container publish` sub-command will perform the following tasks:
1. It will wait for the image to be found in the Red Hat certification API.
2. It will wait for the image scan to be found successful in the Red Hat certification API.
3. It will "publish" the container within the Red Hat certification API.

### Usage

Usage without vault
```shell
./operatorhub container push -a 'api-key-in-keybase' -p `project-id`-r `registry-password-for-quay.io` --dry-run=false
./operatorhub container publish -a 'api-key-in-keybase' -p `project-id` -r `registry-password-for-quay.io` --dry-run=false
```

Usage with vault
```shell
OHUB_GITHUB_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-github" OHUB_REDHAT_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat" VAULT_ADDR='https://vault-server:8200' VAULT_TOKEN=my-token ./bin/operatorhub container publish --enable-vault --dry-run=false
```

### Flags

| Parameter              | Description                                                                                                                               | Environment Variable     | Default           |
|----------------------  |-------------------------------------------------------------------------------------------------------------------------------------------|--------------------------|-------------------|
| `--api-key`            | API key to use when communicating with redhat certification API.                                                                          | `OHUB_API_KEY`           | `""`              |
| `--conf`               | Path to config.yaml file.                                                                                                                 | `OHUB_CONF`              | `"./config.yaml"` |
| `--registry-password`  | Registry password used to communicate with Quay.io.                                                                                       | `OHUB_REGISTRY_PASSWORD` | `""`              |
| `--project-id`         | Red Hat project id within the Red Hat technology portal.                                                                                  | `OHUB_PROJECT_ID`        | `""`              |
| `--dry-run`            | Only validation will be run for the push command if set. The publish command will ensure the image is scanned but will not publish if set.| `OHUB_DRY_RUN`           | `true`            |

## Generate Manifests Command

The `generate-manifests` command will extracts CRDs and RBAC definitions from distribution YAML manifests
(either yaml manifests from flag, or pulled from internet) and generates the files required to publish
a new release to OperatorHub.  These files are written within the following 3 directories:

- community-operators
- certified-operators

### Usage

To generate configuration for a previously released manifest version

```shell
./bin/operatorhub generate-manifests -c config.yaml
```

To generate configuration based on yet unreleased YAML manifests:

```shell
# If using newVersion|prevVersion|stackVersion variables in config.yaml
./bin/operatorhub generate-manifests -c config.yaml -y ../../config/crds.yaml -y ./../config/operator.yaml
```

### Flags

| Parameter         | Description                                                                                       | Environment Variable | Default           |
|-------------------|---------------------------------------------------------------------------------------------------|----------------------|-------------------|
| `--conf`          | Path to config.yaml file.                                                                         | `OHUB_CONF`          | `"./config.yaml"` |
| `--yaml-manifest` | Path(s) to yaml installation manifest files.                                                      | `OHUB_YAML_MANIFEST` | `""`              |
| `--templates`     | Path to the templates directory.                                                                  | `OHUB_TEMPLATES`     | `"./templates"`   |

*IMPORTANT: The operator deployment spec is different from the spec in `operator.yaml` and cannot be automatically extracted from it. Therefore, the deployment spec is hardcoded into the template and should be checked with each new release to ensure that it is still correct.*

## Bundle Command

*This command's sub-commands all requires the output of the `generate-manifests` command to be run*

### Create-PR sub-command

The `bundle create-pr` command will perform the following tasks:
1. Will create a pull request in both `redhat-openshift-ecosystem/certified-operators` and `k8s-operatorhub/community-operators` repositories using the output of both `operatorhub` command, and the `bundle generate` command.

### Usage

- Ensure that the `bundle generate` command has successfully ran
- If using vault, ensure that your personal Github information is contained within vault, including a temporary/expiring Github API token.
  - Your token needs to have the following scopes: `repo`, `workflow`, `read:org`, `read:user`, `user:email`
- Run the `bundle create-pr` command.

Without vault
```shell
./operatorhub bundle create-pr -d . -f 'Your Name' -g 'your-github-token' -u 'your-github-username' -e 'your-github-email'
```

With vault
```shell
OHUB_GITHUB_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-github" OHUB_REDHAT_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat" VAULT_ADDR='https://vault-server:8200' VAULT_TOKEN=my-token ./operatorhub bundle create-pr -d .
```

### Flags

| Parameter                        | Description                                                                                                                               | Environment Variable                | Default           |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------|-------------------|
| `--dir`                          | Directory containing the output of the `generate-manifests` command.                                                                      | `OHUB_DIR`                          | `"./"`            |
| `--conf`                         | Path to config.yaml file.                                                                                                                 | `OHUB_CONF`                         | `"./config.yaml"` |
| `--github-token`                 | User's Github API token.                                                                                                                  | `OHUB_GITHUB_TOKEN`                 | `""`              |
| `--github-username`              | User's Github username.                                                                                                                   | `OHUB_GITHUB_USERNAME`              | `""`              |
| `--github-fullname`              | User's Github fullname.                                                                                                                   | `OHUB_GITHUB_FULLNAME`              | `""`              |
| `--github-email`                 | User's Github email address.                                                                                                              | `OHUB_GITHUB_EMAIL`                 | `""`              |
| `--delete-temp-directory`        | Whether to delete the temporary directory upon completion (useful for debugging).                                                         | `OHUB_DELETE_TEMP_DIRECTORY`        | `true`            |
| `--dry-run`                      | If set, Github forks, and branches will be created within user's remote, but pull requests will not be created.                           | `OHUB_DRY_RUN`                      | `true`            |
