# Manifest Generator

This directory contains a Go program that uses the Helm chart available at `$REPO_ROOT/deploy/eck` to generate manifests for deploying ECK in various configurations. It is also the driver for generating the YAML files used for distributing ECK.

Use the `manifest-gen.sh` helper script to run the Manifest Generator. The `test.sh` script runs through all available profiles and validates the output. Both these scripts are invoked by targets in the `Makefile` at the root of the repository.

## Usage

Update the `appVersion` and CRDs in the Helm chart at `$REPO_ROOT/deploy/eck`.

```sh
./manifest-gen.sh -u
```

Show usage for generate mode.

```sh
./manifest-gen.sh -g --help
```

Generate `all-in-one.yaml`.

```sh
./manifest-gen.sh -g
```

Generate the multi-tenancy manifests.

```sh
./manifest-gen.sh -g --profile=soft-multi-tenancy --set=kubeAPIServerIP=1.2.3.4
```



