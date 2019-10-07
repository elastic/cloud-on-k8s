# Deployer

Deployer is the provisioning tool that aims to be the interface to multiple Kubernetes providers.

At the moment it support GKE and AKS.

Default values for settings are kept in config/plans.yml file. More settings and setting overrides are provided via deployer-config.yml file.

## Typical usage

While in repo root, run once:
```
make dependencies
cd hack/deployer
go build
```

Then, depending on the provider, run the following:

For GKE, with your GCLOUD_PROJECT:
```
cat > config/deployer-config.yml << EOF
id: gke-dev
overrides:
  clusterName: dkowalski-dev-cluster
  gke:
    gCloudProject: GCLOUD_PROJECT
EOF
```

For AKS, with your ACR_NAME and RESOURCE_GROUP:
```
cat > config/deployer-config.yml << EOF
id: aks-dev
overrides:
  clusterName: dkowalski-dev-cluster
  aks:
    resourceGroup: RESOURCE_GROUP
    acrName: ACR_NAME
EOF
``` 


Then, to create, run:
```
./deployer execute
```

Then, to delete, run: 
```
./deployer execute --operation delete
``` 


## CI usage

CI will populate deployer-config with vault login information and deployer will fetch the needed secrets. Secrets will differ depending on the provider chosen.

## CI impersonation

To facilitate testing locally, developers can run "as CI". While the credentials and vault login method are different it does allow fetching the same credentials and logging in as the same service account as CI would. This aims to shorten the dev cycle for CI related work and debugging.

To achieve the above, add the following to your deployer-config.yml, where TOKEN is your GitHub personal access token and VAULT_ADDRESS is the address of your Vault instance.

```
overrides:
  vaultInfo:
    token: TOKEN
    address: VAULT_ADDRESS
  ...
```
