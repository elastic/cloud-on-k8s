# Reattach-PV

This tool can be used to recreate an Elasticsearch cluster by reusing orphaned PersistentVolumes that used to belong to a cluster before it was deleted.

**Warning**: to be used at your own risk. This tool has not been tested extensively with multiple Kubernetes distributions and PersistentVolume providers. You should backup the data in the underlying storage system before attempting to use this tool. Also make sure you perform a dry-run first.

## Expectations

This tool can only be used when the following conditions are met:

1. If re-building a cluster that was deleted, and no longer exists.

* The Elasticsearch resource to re-create does not exist in Kubernetes.
* All PersistentVolumeClaims of the previous cluster do not exist anymore.
* All PersistentVolumes of the previous cluster still exist with the status `Released`.
* The Elasticsearch resource to re-create has the exact same specs as the deleted one. Same cluster name, same node sets, same count, etc.
* The current default kubectl context targets the desired Kubernetes cluster.

2. If building a new cluster, with a new name, from existing unused PVs from a previously deleted, and re-created cluster

* The Elasticsearch resource to create does not exist in Kubernetes. (The previous cluster with the previous name can exist, using new PVs)
* All PersistentVolumes of the previous cluster still exist with the status `Released`.
* The Elasticsearch resource to create has the exact same specs (same node sets, same count, etc.) as the deleted, and re-created one, but with different cluster name.
* The current default kubectl context targets the desired Kubernetes cluster.

## Usage

### To recreate a previously deleted cluster that does not currently exist.

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

Example:

```
# build the binary with a recent Go version
go build
# perform a dry run first
./reattach-pv --elasticsearch-manifest elasticsearch.yml --dry-run
# then, execute again without the dry-run flag
./reattach-pv --elasticsearch-manifest elasticsearch.yml
```

### To create a new cluster, with a new name, from a previously deleted cluster

```
Create a newly-named Elasticsearch cluster by reattaching existing released PersistentVolumes

Usage:
  reattach-pv [flags]

Flags:
      --dry-run                         do not apply any Kubernetes resource change
      --elasticsearch-manifest string   path pointing to the Elasticsearch yaml manifest
  -h, --help                            help for reattach-pv
      --old-elasticsearch-name string   name of previous Elasticsearch cluster (to use existing volumes)
```

Example assuming `clusterA` was accidently deleted, then re-created with new data volumes, and is in use.
Previous PersistentVolumes for `clusterA` still exist, and will be used to build `clusterB`.

```
# build the binary with a recent Go version
go build
# perform a dry run first
# elasticsearch.yaml contains newly-named cluster (ex: `clusterB`)
./reattach-pv --elasticsearch-manifest elasticsearch.yml --dry-run --old-elasticsearch-name clusterA
# then, execute again without the dry-run flag
./reattach-pv --elasticsearch-manifest elasticsearch.yml --old-elasticsearch-name clusterA
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
