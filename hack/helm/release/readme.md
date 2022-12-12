# ECK Helm Chart Releaser

This tool is designed to handle releasing the ECK Helm charts.  It can both release:

1. A single ECK Helm chart (eck-operator)
2. Multiple ECK Helm charts from a directory (eck resources Helm charts), potentially excluding any named Helm charts.

It is also designed to handle releasing to both production, and non-production Helm Chart repositories.

## Buildkite integration

*This section is a work in progress, and has not been tested*

### Releasing ECK Operator Helm Chart

When a semver compatible tag is pushed to git (as during a normal release), the following process will be triggered in Buildkite

1. Release tool will be run with `--charts-dir` pointing directly to `eck-operator` chart directory (ignoring all eck resource Helm charts), and set to release to dev bucket/repo.
2. If successful, the same operation will be run pointing to production bucket/repo.

### Releasing ECK Resources Helm Charts

When a commit is merged to `main` branch, which includes any change to a `*/Chart.yaml` file (detectable via `git diff --name-only HEAD~1`)

1. Release tool will be run with `--charts-dir` pointing directly to `deploy` chart directory (containing all charts, including `eck-operator`), set to release to dev bucket/repo, and with `--excludes` flag set to `eck-operator`.
2. If successful, the same operation will be run pointing to production bucket/repo.

## Running a Release Manually

The following command will execute the steps

* Release all charts contained within the "path/to/deploy" directory.
* Upload all Helm Chart packages to "elastic-helm-charts-dev" GCS bucket using credentials in "gcs-bucket-credentials.json" file.
* Update Helm index file for "https://helm-dev.elastic.co/helm" Helm repository.

```
releaser --charts-dir=path/to/deploy --credentials-file=path/to/gcs-bucket-credentials.json --dry-run=false
```

## Configuration

| Parameter           | Description                                                                                                    | Environment Variable    | Default                            |
|---------------------|----------------------------------------------------------------------------------------------------------------|-------------------------|------------------------------------|
| `--bucket`          | GCS bucket to upload Helm chart packages.                                                                      | `HELM_BUCKET`           | `elastic-helm-charts-dev`          |
| `--charts-dir`      | Full path to directory containing Helm charts to release.                                                      | `HELM_CHARTS_DIR`       | `./deploy`                         |
| `--charts-repo-url` | URL of Helm Charts Repository.                                                                                 | `HELM_CHARTS_REPO_URL`  | `https://helm-dev.elastic.co/helm` |
| `--credentials-file`| Full path to credentials file to use for GCS bucket.                                                           | `HELM_CREDENTIALS_FILE` | `""`                               |
| `--dry-run`         | Will package all Helm charts and process the Helm index, but not upload Helm packages, or update remote index. | `HELM_DRY_RUN`          | `true`                             |
| `--excludes`        | Comma separated list of Helm chart names to exclude.                                                           | `HELM_EXCLUDES`         | `[]`                               |

# TODO

- [x] Handle updating index
- [x] Add dry run?
- [x] Excludes
- [x] Release single chart?
- [ ] New logger with info/debug capabilities?
- [x] Tests
- [x] Manual Testing
- [x] Better organization, and naming of funcs
- [x] Remove writing to fake gcs bucket
- [ ] Validation of chart repo url + bucket combinations
- [ ] Documentation
- [ ] Personal review
- [ ] BK integration (how to expose env vars)
- [ ] BK testing