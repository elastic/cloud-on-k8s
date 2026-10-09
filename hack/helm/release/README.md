# ECK Helm charts releaser

Tool to release ECK Helm charts.

```sh
Usage:
  release [flags]

Examples:
  release --env=prod --charts-dir=./deploy --dry-run=false
  release --env=prod --charts-dir=./deploy --dry-run=false --skip-chart-repo
  release --env=prod --charts-dir=./deploy --dry-run=false --skip-oci-registry

Flags:
      --charts-dir string                Directory which contains Helm charts to release (env: HELM_CHARTS_DIR) (default "./deploy")
      --credentials-file string          Path to GCS credentials JSON file (env: HELM_CREDENTIALS_FILE) (default "/tmp/credentials.json")
  -d, --dry-run                          Do not upload files to bucket, update Helm index, or push to OCI registry (env: HELM_DRY_RUN) (default true)
      --enable-vault                     Read 'credentials-file' and the OCI registry credentials from Vault (requires VAULT_ADDR and VAULT_TOKEN). When disabled, the local Docker/Helm registry login is used (env: HELM_ENABLE_VAULT) (default true)
      --env string                       Environment in which to release Helm charts ('dev' or 'prod') (env: HELM_ENV) (default "dev")
  -f, --force                            Upload artifacts even if they already exist (env: HELM_FORCE)
  -h, --help                             help for release
  -k, --keep-tmp-dir                     Keep temporary directory which contains the Helm charts ready to be published (env: HELM_KEEP_TMP_DIR)
      --oci-charts-digests-file string   Path to a file where pushed OCI chart digest refs are written, one per line (e.g. registry/chart:version@sha256:...). Empty to skip (env: HELM_OCI_CHARTS_DIGESTS_FILE)
      --skip-chart-repo                  Skip uploading to the GCS bucket and updating the Helm index. Useful when only OCI publishing is needed (env: HELM_SKIP_CHART_REPO)
      --skip-oci-registry                Skip pushing charts to the OCI registry. Useful when only GCS publishing is needed (env: HELM_SKIP_OCI_REGISTRY)
```

Each environment has a fixed set of release targets:

| Env    | GCS bucket                | Helm repository                    | OCI registry                             |
|--------|---------------------------|------------------------------------|------------------------------------------|
| `dev`  | `elastic-helm-charts-dev` | `https://helm-dev.elastic.co/helm` | `docker.elastic.co/eck-charts-snapshots` |
| `prod` | `elastic-helm-charts`     | `https://helm.elastic.co/helm`     | `docker.elastic.co/eck-charts`           |

With `--enable-vault` (the default), the OCI registry credentials are read from Vault (`docker-registry-elastic`) and kept in memory. With `--enable-vault=false`, the local Helm registry or Docker login is used (e.g. via `docker login docker.elastic.co`).

Charts are pushed to dedicated OCI namespaces, separate from the `eck` and `eck-snapshots` namespaces that hold the operator container images. Each repository name equals the chart name (e.g. `docker.elastic.co/eck-charts/eck-operator:1.0.0`), which Helm requires when a chart is consumed as a dependency of another chart.

### Structure

ECK Helm charts are grouped under 2 parent charts: `eck-operator` and `eck-stack`.

```
.
├── eck-operator
│   └── charts
│       └── eck-operator-crds
│
├── eck-stack
│   ├── charts
│   │   ├── eck-agent
│   │   ├── eck-beats
│   │   ├── eck-elasticsearch
│   │   ├── eck-fleet-server
│   │   └── eck-kibana
│   │   └── eck-logstash
```

All subcharts are managed locally through their parent's `charts/` directory.

They are released and therefore usable independently of their parent.
