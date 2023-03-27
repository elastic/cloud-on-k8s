# Continuous integration

### Structure

We are using Jenkins as CI runner and keep its configuration as code in the repo. The address of the instance we use is https://devops-ci.elastic.co/view/cloud-on-k8s/.

There are few layers in most of our jobs:
 
1. [Job definition](jobs) - description of the job.
2. [Jenkinsfile](pipelines) - loads vault credentials, sets up configuration. 
3. [CI makefile](Makefile) - creates container to run CI in, consolidates dev and CI setups.
4. [Main makefile](../Makefile) - contains logic, delegates to specific tools as needed.
5. tools - e.g. for [e2e test running](../test/e2e) and [cluster provisioning](../hack/deployer).

### Local repro

For debugging and development purposes it's possible to run CI jobs from dev box. It requires minimal setup and it mirrors CI closely, starting at CI makefile layer.

Once, run:
```
# fill out:
export VAULT_ADDR=YOUR_VAULT_INSTANCE_ADDRESS
export GITHUB_TOKEN=YOUR_PERSONAL_ACCESS_TOKEN
``` 

Per repro, depending on the job, set up `.env` and `deployer-config.yml` files by using [pipeline-gen](.buildkite/e2e/pipeline-gen) and[set-deployer-config.sh](.buildkite/scripts/test/set-deployer-config.sh). Example:

```sh
> .buildkite/e2e/pipeline-gen/pipeline-gen -f p=gke,s=8.6.2 -e | tee .env
E2E_JSON=true
GO_TAGS=release
export LICENSE_PUBKEY=/Users/krkr/dev/src/github.com/elastic/cloud-on-k8s/.ci/license.key
E2E_IMG=docker.elastic.co/eck-dev/eck-e2e-tests:2.8.0-SNAPSHOT-f01854af
OPERATOR_IMAGE=docker.elastic.co/eck-dev/eck-operator-krkr:2.8.0-SNAPSHOT-f01854af
E2E_PROVIDER=gke
TEST_OPTS=-race
TEST_LICENSE=/Users/krkr/dev/src/github.com/elastic/cloud-on-k8s/.ci/test-license.json
MONITORING_SECRETS=
PIPELINE=e2e/gke
CLUSTER_NAME=eck-e2e-gke-dzau-0
BUILD_NUMBER=0
E2E_STACK_VERSION=8.6.2

> export $(cat .env | xargs)

> .buildkite/scripts/test/set-deployer-config.sh

> make -C .ci TARGET="run-deployer e2e-run" ci
```

The CI Makefile will take care of setting up correct credentials in the `deployer-config.yml` file. For more details about settings in this file, see [deployer](/hack/deployer/README.md#advanced-usage).

This will run e2e tests using the same:
1. container
1. credentials
1. settings
1. call path

as the CI job.

### CI container

You can build and run CI container interactively with:

```
make -C .ci ci-interactive
```
