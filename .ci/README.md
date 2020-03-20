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

Per repro, depending on the job, set up `.env` and `deployer-config.yml` files by using [setenvconfig](setenvconfig) invocation from the respective Jenkinsfile. The script will prompt for any missing environment variables that are required for a given job. See examples below. 

Test the `cloud-on-k8s-e2e-tests-master` job:
```sh
.ci/setenvconfig e2e/master
make -C .ci get-test-artifacts TARGET=ci-build-operator-e2e-run ci
```

Test the `cloud-on-k8s-e2e-tests-stack-versions` job:
```sh
JKS_PARAM_OPERATOR_IMAGE=docker.elastic.co/eck-snapshots/eck-operator:1.0.1-SNAPSHOT-2020-02-05-7892889 \
  .ci/setenvconfig e2e/stack-versions eck-75-dev-e2e 7.5.1
make -C .ci get-test-license get-elastic-public-key TARGET=ci-e2e ci
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
