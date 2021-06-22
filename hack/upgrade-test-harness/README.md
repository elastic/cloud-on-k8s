ECK Upgrade Test Harness
========================

Test harness to exercise the upgrade process for ECK releases.


Usage
-----

```
go run main.go --from-release=alpha --to-release=upcoming
```

CAUTION: Running the test harness could wipe out existing CRDs and operator deployments. It should ideally be run on a fresh, non-production cluster.

```
Flags:
      --conf-file string                   Path to the file containing test params (default "conf.yaml")
      --from-release string                Release to start with (alpha, beta, v101, v112, upcoming) (default "alpha")
      --log-level string                   Log level (DEBUG, INFO, WARN, ERROR) (default "INFO")
      --retry-count uint                   Number of retries (default 5)
      --retry-delay duration               Delay between retries (default 30s)
      --retry-timeout duration             Time limit for retries (default 5m0s)
      --skip-cleanup                       Skip cleaning up after test run
      --to-release string                  Release to finish with (alpha, beta, v101, v112, upcoming) (default "upcoming")
      --upcoming-release-crds string       YAML file for installing the CRDs for the upcoming release (default "../../config/crds.yaml")
      --upcoming-release-operator string   YAML file for installing the operator for the upcoming release (default "../../config/operator.yaml")

In addition, common kubectl flags such as "-n" can be provided. Invoke with "--help" to see all available flags.
```

NOTE: If the `upcoming` release is being tested, ensure that the correct container image is pushed to the container registry before running the test.


Adding a new release
--------------------

- Create a directory under `testdata` following the existing naming convention.
- Add the `crds.yaml` as `crds.yaml` to the release directory.
- Add the `operator.yaml` as `install.yaml` to the release directory.
- Add resource definitions to a file named `stack.yaml` in the release directory. Resource names must match the name of the release.
- Update `conf.yaml` and add the new release to the correct position in the `testParam` list.
