# Buildkite pipeline generator for e2e testing

Generates Buildkite e2e-tests pipelines to stdout.

## multi-group via stdin for nightly builds

```sh
cat <<E2E | pipeline-gen | tee pipeline.yml
# list of groups to organize all tests
- label: stack
  # variables common to all variants
  fixed:
    E2E_PROVIDER: gke
    TESTS_MATCH: TestSmoke
  # list of variables for each variant
  mixed:
    - E2E_STACK_VERSION: "8.6.1"
    - E2E_STACK_VERSION: "8.7.0-SNAPSHOT"
      BUILD_LICENSE_PUBKEY: dev
    - E2E_PROVIDER: kind
      E2E_STACK_VERSION=8.7.0-SNAPSHOT
      BUILD_LICENSE_PUBKEY=dev

- label: TestSmoke
  fixed:
    TESTS_MATCH: TestSmoke
  mixed:
    - E2E_PROVIDER: eks
    - E2E_PROVIDER: aks
E2E
```

This will generate a pipeline that runs the deployer and the e2e-tests in 5 environments:
- E2E_PROVIDER=gke E2E_STACK_VERSION=8.6.1
- E2E_PROVIDER=gke E2E_STACK_VERSION=8.7.0-SNAPSHOT BUILD_LICENSE_PUBKEY=dev
- E2E_PROVIDER=kind E2E_STACK_VERSION=8.7.0-SNAPSHOT BUILD_LICENSE_PUBKEY=dev
- TESTS_MATCH=TestSmoke E2E_PROVIDER=eks
- TESTS_MATCH=TestSmoke E2E_PROVIDER=aks

### single group via flags for triggers from PR comment

```sh
pipeline-gen -f E2E_PROVIDER=gke,TESTS_MATCH=TestSmoke -m E2E_STACK_VERSION=8.5.0,E2E_STACK_VERSION=8.6.0 | tee pipeline.yml
```

`^` is the separator for declaring multiple variables per combination in the `--mixed` flag.

```sh
pipeline-gen -f TESTS_MATCH=TestSmoke -m E2E_PROVIDER=gke^E2E_STACK_VERSION=8.5.0,E2E_PROVIDER=kind^E2E_STACK_VERSION=8.6.0 | tee pipeline.yml
```

### single run and output .env for local execution

```sh
pipeline-gen -e -f E2E_PROVIDER=gke,DEPLOYER_K8S_VERSION=1.23,E2E_STACK_VERSION=8.6.0,TESTS_MATCH=TestSmoke | tee ../../../.env
```

### 'p'k's't' shortcuts for the most used variables

```sh
pipeline-gen -f p=gke,k=1.23,t=TestSmoke -m s=8.5.0,s=8.6.0 | tee pipeline.yml
pipeline-gen -e -f p=gke,k=1.23,s=8.6.0,t=TestSmoke | tee ../../../.env
```
