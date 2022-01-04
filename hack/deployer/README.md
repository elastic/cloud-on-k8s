# Deployer

Deployer is the provisioning tool that aims to be the interface to multiple Kubernetes providers. Currently, it supports GKE and AKS.

## Typical usage

### Provision

* GKE

  * Install [Google Cloud SDK](https://cloud.google.com/sdk/install)
  * Install Google Cloud SDK beta components by running `gcloud components install beta`
  * Make sure that container registry authentication is correctly configured as described [here](https://cloud.google.com/container-registry/docs/advanced-authentication)
  * Set `GCLOUD_PROJECT` to the name of the GCloud project you wish to use
  * (optional) Set `CLOUDSDK_CONFIG` to a directory which should be used for gcloud SDK if you don't want to have the default one overwritten.
  * Run from the [project root](/):

    ```bash
    make switch-gke bootstrap-cloud
    ```

* AKS

  * Install [Azure CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest)
  * Set `RESOURCE_GROUP` to the name of the Resource Group you wish to deploy in
  * Run from the [project root](/):

    ```bash
    make switch-aks bootstrap-cloud
    ```
* Kind
  * No need to install the Kind CLI. Deployer will do that for you and run Kind inside a Docker container without changing the host system.
  * Run from the [project root](/):

    ```bash
    make switch-kind bootstrap-cloud
    ```

  * This will give you a working Kind cluster based on default values. See [Advanced usage](#advanced-usage) on how to tweak these configuration defaults if the need arises. Relevant parameters for Kind are: `client_version` which is the version of Kind to use. Make sure to check the [Kind release notes](https://github.com/kubernetes-sigs/kind/releases) when changing the client version and make sure `kubernetesVersion` and `client_version` are compatible. `kind.nodeImage` allows you to use a specific Kind node image matching your chosen Kind version. Again, the [Kind release notes](https://github.com/kubernetes-sigs/kind/releases) list the compatible pre-built node images for each version. `kind.ipFamily` allows you to switch between either an IPv4 or IPv6 network setup.

### Deprovision

```bash
make delete-cloud
```

## Advanced usage

Deployer uses two config files:

* `config/plans.yml` - to store defaults/baseline settings for different use cases (different providers, CI/dev)
* `config/deployer-config-*.yml` - to "pick" on of the predefined configs from config/plans.yml and allow overriding settings.

You can adjust many parameters that clusters are deployed with. Exhaustive list is defined in [settings.go](runner/settings.go).

Running `make switch-*` (eg. `make-switch-gke`) changes the current context. Running `make create-default-config` generates `config/deployer-config-*.yml` file for the respective provider using environment variables specific to that providers configuration needs. After the file is generated, you can make edit it to suit your needs and run `make bootstrap-cloud` to deploy. Currently chosen provider is stored in `config/provider` file.

You can run deployer directly (not via Makefile in repo root). For details run:

```bash
./deployer help
```
