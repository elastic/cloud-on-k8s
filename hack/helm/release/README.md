# ECK Helm charts releaser

Tool to release ECK Helm charts.

```sh
Usage:
  release [flags]

Examples:
  release --env=prod --charts-dir=./deploy --dry-run=false

Flags:
      --charts-dir string         Directory which contains Helm charts to release (env: HELM_CHARTS_DIR) (default "./deploy")
      --credentials-file string   Path to GCS credentials JSON file (env: HELM_CREDENTIALS_FILE) (default "/tmp/credentials.json")
  -d, --dry-run                   Do not upload files to bucket, or update Helm index (env: HELM_DRY_RUN) (default true)
      --enable-vault              Read 'credentials-file' from Vault (requires VAULT_ADDR and VAULT_TOKEN) (env: HELM_ENABLE_VAULT) (default true)
      --env string                Environment in which to release Helm charts ('dev' or 'prod') (env: HELM_ENV) (default "dev")
  -h, --help                      help for release
```

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
