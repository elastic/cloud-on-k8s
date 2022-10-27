# Reattach-PV

This tool can be used to recreate an Elasticsearch cluster by reusing orphaned PersistentVolumes that used to belong to a cluster before it was deleted.

**Warning**: to be used at your own risk. This tool has not been tested extensively with multiple Kubernetes distributions and PersistentVolume providers. You should backup the data in the underlying storage system before attempting to use this tool. Also make sure you perform a dry-run first.

## Expectations

This tool can only be used when the following conditions are met:

* The Elasticsearch resource to re-create does not exist in Kubernetes.
* All PersistentVolumes of the deleted cluster still exist with the status `Released`.
* The Elasticsearch resource to re-create has the exact same nodeSet specifications as the deleted one (same nodeSet names and counts).
* The current default kubectl context targets the desired Kubernetes cluster.

The Elasticsearch resource to be recreated can have the same name as the deleted one or a new name. In the second case, you must provide the name of the deleted cluster through the flag `--old-elasticsearch-name`.

## Usage

```
Recreate an Elasticsearch cluster by reattaching existing released PersistentVolumes

Usage:
  reattach-pv [flags]

Flags:
      --dry-run                         do not apply any Kubernetes resource change
      --elasticsearch-manifest string   path pointing to the Elasticsearch yaml manifest
  -h, --help                            help for reattach-pv
      --old-elasticsearch-name string   name of previous Elasticsearch cluster (to use existing volumes)
```

Example assuming cluster-A was accidently deleted:

```
# build the binary with a recent Go version
go build

# re-create the cluster with the same name
./reattach-pv --elasticsearch-manifest cluster-A.yml --dry-run

# or re-create the cluster with a new name
./reattach-pv --elasticsearch-manifest cluster-B.yml --old-elasticsearch-name cluster-A --dry-run

# if everything seems ok, execute one of the 2 previous commands again without the dry-run flag
```

## How it works

This tool basically does the following:

* Ensure the Elasticsearch resource and the corresponding PersistentVolumeClaims do not exist in the APIServer.
* Generate the list of PersistentVolumeClaims that would normally be created for this Elasticsearch cluster.
* Retrieve the list of existing Released PersistentVolumes. Match their `claimRef` to the generated PersistentVolumeClaims, based on their name.
* Create the expected PersistentVolumeClaims, with a status set to `Bound`.
* Update the existing PersistentVolumes to reference the newly created PersistentVolumeClaims.
* Create the Elasticsearch resource. The created PersistentVolumeClaims will automatically be used for the Elasticsearch Pods, since they have the correct name convention.

## Limitations

* PersistentVolumeClaims are not created the exact same way they would normally be created by the StatefulSet controller. Especially, they don't have the usual annotations and labels.
* PersistentVolumeClaims are not created with an OwnerReference pointing to the Elasticsearch resource, because they are created before that resource. Therefore, they will not be automatically removed upon Elasticsearch resource deletion.
