# Deployer

Deployer is the provisioning tool that aims to be the interface to multiple Kubernetes providers. It supports GKE, AKS, EKS, OCP, Kind, and K3d.

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

* EKS

  * Install [eksctl](https://github.com/eksctl-io/eksctl?tab=readme-ov-file#installation)
  * Set `AWS_PROFILE` to the profile you wish to use
  * Ensure your AWS credentials are refreshed before running the bootstrap command
  * Run from the [project root](/):

    ```bash
    make switch-eks bootstrap-cloud
    ```

  * If wanting to use a different region (default is eu-west-2), set `overrides.eks.region` in `hack/deployer/config/deployer-config-eks.yml`

* Kind
  * No need to install the Kind CLI. Deployer will do that for you and run Kind inside a Docker container without changing the host system.
  * Run from the [project root](/):

    ```bash
    make switch-kind bootstrap-cloud
    ```

  * This will give you a working Kind cluster based on default values. See [Advanced usage](#advanced-usage) on how to tweak these configuration defaults if the need arises. Relevant parameters for Kind are: `client_version` which is the version of Kind to use. Make sure to check the [Kind release notes](https://github.com/kubernetes-sigs/kind/releases) when changing the client version and make sure `kubernetesVersion` and `client_version` are compatible. `kind.nodeImage` allows you to use a specific Kind node image matching your chosen Kind version. Again, the [Kind release notes](https://github.com/kubernetes-sigs/kind/releases) list the compatible pre-built node images for each version. `kind.ipFamily` allows you to switch between either an IPv4 or IPv6 network setup.

* K3d
  * No need to install the K3d CLI. Deployer will do that for you and run K3d inside a Docker container without changing the host system.
  * Run from the [project root](/):

    ```bash
    make switch-k3d bootstrap-cloud
    ```

  * This will give you a working K3d cluster based on default values. See [Advanced usage](#advanced-usage) on how to tweak these configuration defaults if the need arises. Relevant parameters for K3d are: `clientImage` which is the version of K3d to use and `nodeImage` which is the version of `k3s` that runs on the nodes, which also defines the Kubernetes version. Make sure to check the [K3d release notes](https://github.com/k3d-io/k3d/releases) when changing the client image and make sure `nodeImage` and `clientImage` are compatible.

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

## Bucket Provisioning

Deployer can optionally create a cloud storage bucket alongside the Kubernetes cluster. The bucket credentials are stored in a Kubernetes Secret, ready for use by applications like Elasticsearch Stateless.

To enable bucket provisioning, add a `bucket` section to your plan or deployer config:

```yaml
bucket:
  name: "{{ .ClusterName }}-development"
  storageClass: standard
  secret:
    name: "{{ .ClusterName }}-bucket-secret"
    namespace: default
```

The `name` and `secret.name`/`secret.namespace` fields support Go template variables (e.g., `{{ .ClusterName }}`).

### Provider-specific behavior

**GKE / OCP (Google Cloud Storage)**

Creates a GCS bucket and a service account with `roles/storage.objectAdmin` scoped to that bucket. The Secret contains:
- `gcs.client.default.credentials_file` — the service account JSON key

OCP clusters run on GCP, so they use the same GCS bucket provisioning as GKE.

**AKS (Azure Blob Storage)**

Creates an Azure Storage account and a blob container named `data`. The Secret contains:
- `azure.client.default.account` — the storage account name
- `azure.client.default.sas_token` — a SAS token valid for 1 year

**EKS (Amazon S3)**

Creates an S3 bucket and an IAM user with access keys. The Secret contains:
- `access-key-id` — the IAM access key ID
- `secret-access-key` — the IAM secret access key
- `bucket` — the bucket name
- `region` — the AWS region

For EKS, additional S3-specific settings are required to specify the IAM path and managed policy:

```yaml
bucket:
  name: "{{ .ClusterName }}-development"
  storageClass: STANDARD
  secret:
    name: "{{ .ClusterName }}-bucket-secret"
    namespace: default
  s3:
    iamUserPath: "/path/to/iam/users/"
    managedPolicyARN: "arn:aws:iam::123456789012:policy/path/to/policy"
```

- `iamUserPath` — the IAM path under which the storage user is created (must match your IAM policy constraints)
- `managedPolicyARN` — the ARN of a pre-existing managed policy that grants S3 access to the bucket

### Cleanup

Buckets and their associated cloud resources (IAM users, service accounts, storage accounts) are automatically deleted when running `make delete-cloud`. The Kubernetes Secret is deleted along with the cluster.
