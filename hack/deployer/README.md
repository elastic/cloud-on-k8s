# Deployer

Deployer is the provisioning tool that aims to be the interface to multiple Kubernetes providers. Currently, it supports GKE and AKS.

## Typical usage

### Provision

* GKE

  * Install [Google Cloud SDK](https://cloud.google.com/sdk/install)
  * Install Google Cloud SDK beta components by running `gcloud components install beta`
  * Make sure that container registry authentication is correctly configured as described [here](https://cloud.google.com/container-registry/docs/advanced-authentication)
  * Set `GCLOUD_PROJECT` to the name of the GCloud project you wish to use
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
