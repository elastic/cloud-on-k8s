# ECK Helm Chart Releaser

This tool is designed to handle releasing the ECK Helm charts.  It can both release:

1. A single ECK Helm chart (eck-operator)
2. Multiple ECK Helm charts from a directory (eck resources Helm charts), potentially excluding any named Helm charts.

It is also designed to handle releasing to both production, and non-production Helm Chart repositories.

## Buildkite integration


### Releasing ECK Operator Helm Chart

When a semver compatible tag is pushed to git (as during a normal release), the following process will be triggered in Buildkite

1. Release tool will be run with `--charts-dir` pointing directly to `eck-operator` chart directory (ignoring all eck resource Helm charts), and set to release to dev bucket/repo.
2. If successful, the same operation will be run pointing to production bucket/repo.

#### Manually releasing the ECK Operator Helm Chart

To manually publish the ECK Operator Helm chart and it's associated CRD chart, run the below `curl` command.

*Note if there are changes to the Helm tooling that are still in a PR status, you'll need to add "pull_request_repository":"git://github.com/your-username/cloud-on-k8s","pull_request_id":"pr-id" to the below API call.*

```
  curl --request POST --url https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator/builds \
    --header 'Authorization: Bearer your-bk-token' --header 'Content-Type: application/json' \
    --data '{
        "commit": "HEAD",
        "branch": "main",
        "env": {
          "HELM_DRY_RUN": "false",
          "HELM_BRANCH": "2.6"
        },
        "message": "release eck-operator helm chart"
    }'
```

You can then track the progress of the build in [Buildkite](https://buildkite.com/elastic/cloud-on-k8s-operator)

### Releasing ECK Resources Helm Charts

When a commit is merged to `main` branch, which includes any change to a `*/Chart.yaml` file (detectable via `git diff --name-only HEAD~1`)

1. Release tool will be run with `--charts-dir` pointing directly to `deploy` chart directory (containing all charts, including `eck-operator`), set to release to dev bucket/repo, and with `--excludes` flag set to `eck-operator`.
2. If successful, the same operation will be run pointing to production bucket/repo.

#### Manually releasing the ECK Resources Helm Charts

To manually publish the ECK Resources Helm charts, run the below `curl` command.

*Note if there are changes to the Helm tooling that are still in a PR status, you'll need to add "pull_request_repository":"git://github.com/your-username/cloud-on-k8s","pull_request_id":"pr-id" to the below API call.*

```
  curl --request POST --url https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator/builds \
    --header 'Authorization: Bearer your-bk-token' --header 'Content-Type: application/json' \
    --data '{
        "commit": "HEAD",
        "branch": "main",
        "env": {
          "HELM_DRY_RUN": "false",
          "HELM_BRANCH": "2.6"
        },
        "message": "release eck-resources helm charts"
    }'
```

You can then track the progress of the build in [Buildkite](https://buildkite.com/elastic/cloud-on-k8s-operator)

## Running a Release Manually

The following command will execute the steps

* Release all charts contained within the "path/to/deploy" directory.
* Upload all Helm Chart packages to "elastic-helm-charts-dev" GCS bucket using credentials in "gcs-bucket-credentials.json" file.
* Update Helm index file for "https://helm-dev.elastic.co/helm" Helm repository.

```
releaser --env=dev --charts-dir=path/to/deploy --credentials-file=path/to/gcs-bucket-credentials.json --dry-run=false
```

## Configuration

| Parameter           | Description                                                                                                    | Environment Variable    | Default                            |
|---------------------|----------------------------------------------------------------------------------------------------------------|-------------------------|------------------------------------|
| `--env`             | Environment in which to upload Helm chart packages.                                                            | `HELM_ENV`              | `dev`                              |
| `--charts-dir`      | Full path to directory containing Helm charts to release.                                                      | `HELM_CHARTS_DIR`       | `./deploy`                         |
| `--credentials-file`| Full path to credentials file to use for GCS bucket.                                                           | `HELM_CREDENTIALS_FILE` | `"/tmp/credentials.json"`                               |
| `--dry-run`         | Will package all Helm charts and process the Helm index, but not upload Helm packages, or update remote index. | `HELM_DRY_RUN`          | `true`                             |
| `--excludes`        | Comma separated list of Helm chart names to exclude.                                                           | `HELM_EXCLUDES`         | `[]`                               |
