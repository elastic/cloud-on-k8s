# Continuous integration

### Structure

We are using Jenkins as CI runner and keep its configuration as code in the repo. The address of the instance we use is https://devops-ci.elastic.co/view/cloud-on-k8s/.

There are few layers in most of our jobs:
 
1. [Job definition](../../.ci/jobs) - description of the job.
2. Jenkinsfile (e.g.: [e2e/Jenkinsfile](e2e/Jenkinsfile)) - loads vault credentials, sets up configuration. 
3. [CI makefile](Makefile) - creates container to run CI in, consolidates dev and CI setups.
4. [dev makefile](../../Makefile) - contains logic, delegates to specific tools as needed.
5. tools - e.g. for [e2e test running](../../test/e2e) and [cluster provisioning](../../hack/deployer).

### Local repro

For debugging and development purposes it's possible to run CI jobs from dev box. It requires minimal setup and it mirrors CI closely, starting at CI makefile layer.

Once, run:
```
export BUILD_TAG=local-ci-$(USER//_)

# fill out:
export GCLOUD_PROJECT=YOUR_GCLOUD_PROJECT
export VAULT_ADDR=YOUR_VAULT_INSTANCE_ADDRESS
export GITHUB_TOKEN=YOUR_PERSONAL_ACCESS_TOKEN
``` 

Per repro, depending on the job, set up .env and deployer-config.yml files. E.g.: to repro e2e tests run, look at its [Jenkinsfile](e2e/Jenkinsfile) and rerun the script locally in repo root: 
```
cat >.env <<EOF
GCLOUD_PROJECT = "$GCLOUD_PROJECT"
REGISTRY = eu.gcr.io
REPOSITORY = "$GCLOUD_PROJECT"
SKIP_DOCKER_COMMAND = false
IMG_SUFFIX = -ci
EOF

cat >deployer-config.yml <<EOF
id: gke-ci
overrides:
  kubernetesVersion: "1.12"
  clusterName: $BUILD_TAG
  vaultInfo:
    address: $VAULT_ADDR
    roleId: $VAULT_ROLE_ID
    secretId: $VAULT_SECRET_ID
  gke:
    gCloudProject: $GCLOUD_PROJECT
EOF

make -C build/ci TARGET=ci-e2e ci
```

CI makefile will take care of setting up correct credentials in the .env and deployer-config.yml file.

This will run e2e test using the same:
1. container
1. credentials
1. settings
1. call path

as the CI job.

### CI container

You can build and run CI container interactively with:

```
make -C build/ci ci-interactive
```
