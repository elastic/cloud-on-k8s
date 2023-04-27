# ECK Helm charts releaser

Tool to handle releasing ECK Helm charts.

```sh
Usage:
  release [flags]

Examples:
  release --charts-dir=./deploy --dry-run=false

Flags:
      --charts-dir string         Directory which contains Helm charts to release (env: HELM_CHARTS_DIR) (default "./deploy")
      --credentials-file string   Path to google credentials json file (env: HELM_CREDENTIALS_FILE) (default "/tmp/credentials.json")
  -d, --dry-run                   Do not upload files to bucket, or update helm index (env: HELM_DRY_RUN) (default true)
      --enable-vault              Read 'credentials-file' from Vault (requires VAULT_ADDR and VAULT_TOKEN) (env: HELM_ENABLE_VAULT) (default true)
      --env string                Environment in which to release Helm charts (env: HELM_ENV) (default "dev")
  -h, --help                      Help
```

### Limitations

Only supports publishing parent charts that have local dependencies in the `charts/` directory.
