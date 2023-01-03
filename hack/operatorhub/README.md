# OperatorHub Release Operations Commands

Set of commands to simplify operations required when releasing new versions of the operator for openshift/operatorhub, and community/certified operators. 

These commands include

- Pushing operator image to Quay.io
- Publishing eck operator container image within redhat catalog
- Generate Operator Lifecycle Manager format files
- Generate Operator bundle metadata (wrapper for [OPM](https://github.com/operator-framework/operator-registry/tree/master/cmd/opm) command)
- Publish draft pull requests to both https://github.com/redhat-openshift-ecosystem/certified-operators and https://github.com/k8s-operatorhub/community-operators

## Commands Overview

* container - parent command
  * push - push operator container to quay.io
  * publish - publish operator container within redhat certification API
* generate-manifests - generate operator lifecycle manager format files
* bundle - parent command
  * generate - generate operator metadata for publishing on openshift operator hub
  * create-pr - perform all git operations and create pull requests for community and certified operator repositories

## Commands

### Container Push

The `container push` sub-command will perform the following tasks:
1. Determine if there is an image in the [redhat certification API](https://catalog.redhat.com/api/containers/v1) that has the given `tag`, using the provided `project-id`.
2. If image is already found, nothing is done without using the `force` flag.
3. If image not found, or `force` flag set, will push `docker.elastic.co/eck/eck-operator-ubi8:$(tag)` to `quay.io` docker registry, tagged as `quay.io/redhat-isv-containers/$(project-id):$(tag)`.

### Container Publish

The `container publish` sub-command will perform the following tasks:
1. It will wait for the image to be found in the Red Hat certification API.
2. It will wait for the image scan to be found successful in the Red Hat certification API.
3. It will "publish" the container within the Red Hat certification API.

#### Usage

*notes*
- `api-key` is the Red Hat API key that exists within the keybase application.
- `registry-password` is the quay.io password that can be obtained by logging into [redhat connect](https://connect.redhat.com) and clicking [Push Image Manually](https://connect.redhat.com/projects/$(project-id)/images/upload-image), which will then show you a `Registry Key`.
- `project-id` is the Red Hat certification project ID found within [redhat connect](https://connect.redhat.com).
- All of these can be contained within a secret in vault, and be pulled directly from vault.

Usage without vault
```shell
./operatorhub container push -a 'api-key-in-keybase' -p `project-id` -t 2.6.0 -r `registry-password-for-quay.io` --dry-run=false
./operatorhub container publish -a 'api-key-in-keybase' -p `project-id` -t 2.6.0 -r `registry-password-for-quay.io` --dry-run=false
```

Usage with vault
```shell
OHUB_TAG=2.6.0-bc2 OHUB_GITHUB_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-github" OHUB_REDHAT_VAULT_SECRET="secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat" VAULT_ADDR='https://vault-server:8200' VAULT_TOKEN=my-token ./bin/operatorhub container publish --enable-vault --dry-run=false
```

Example vault secret
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

### Bundle

*This command requires the output of the `generate-manifests` command to be run*

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
  -r, --registry-password string   registry key used to communicate with redhat docker registry (REGISTRY_PASSWORD)
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
./redhat all -a 'api-key-in-keybase' -r 'registry-password' \
  -f 'Your Name' -g 'your-github-token' -u 'your-github-username' -e 'your-github-email' \
  -t '1.9.2' -s '7.16.0' -V '1.9.1' -y ../../config/crds.yaml -y ./../config/operator.yaml
```

# TODO

- [ ] Test
- [ ] use consts
- [ ] Update readme since operations changed
- [ ] Comment all funcs
- [ ] What about the 'v' prefix we're going to add?
- [ ] Do all the things in 'cmd/*' package have to be exported now?
- [ ] "%s is required" is redundant.
- [ ] name: elastic-cloud-eck.v2.6.0-bc2 in community-operators/tag/*.csv.yaml is already prefixed with 'v'
- [ ] document that `make generate-crds-v1` needs to be run prior to this.
- [ ] Move loginToRegistry to preflight pkg
- [ ] getFirstValidImage flow.... called after getImages.  Can this be optimized?