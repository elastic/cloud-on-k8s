# Deployer

Deployer is the provisioning tool that aims to be the interface to multiple Kubernetes providers.

At the moment it support GKE and AKS.

Default values for settings are kept in config/plans.yml file. More settings and setting overrides are provided via run-config.yml file.

## Typical usage

While in:
```
$ pwd
/go/src/github.com/elastic/cloud-on-k8s/operators
```

Run once with your GCLOUD_PROJECT:
```
make dep-vendor-only
cd hack/deployer
go build
cat > run-config.yml << EOF
id: gke-ci
overrides:
  clusterName: dkowalski-dev-cluster
  serviceAccount: false
  gke:
    gCloudProject: GCLOUD_PROJECT
EOF
```

Then, to create, run:
```
./deployer
```

Then, to delete, add 
```
overrides:
  operation: delete
...
``` 
to your run-config.yml and run `./deployer` again.


## CI usage

CI will populate run-config with vault login information and deployer will fetch the needed secrets. Secrets will differ depending on the provider chosen.

## CI impersonation

To facilitate testing locally, developers can run "as CI". While the credentials and vault login method are different it does allow fetching the same credentials and logging in as the same service account as CI would. This aims to shorten the dev cycle for CI related work and debugging.

To achieve the above, add the following to your run-config.yml, where TOKEN is your GitHub personal access token.

```
overrides:
  vaultInfo:
    token: TOKEN
  ...
```