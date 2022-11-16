# 4. Licensing

* Status:  accepted 
* Deciders: k8s Team
* Date: 2019-02-12


## Context and Problem Statement

Purpose of this decision record is to outline an implementation for license management for Elasticsearch clusters managed by the Elastic k8s operator.

## Decision Drivers <!-- optional -->

* gold/platinum level licenses can only be applied to clusters using internal TLS
* user applying the license needs to have `manage` privileges if security features are enabled (which is always the case)
* enterprise licenses should be shared between multiple clusters
* in some use cases we might want to isolate enterprise licenses from clusters by using a different namespace 

## Considered Options

### Option 1: Directly applied license

Simplest option. We support a new resource type `ClusterLicense` which is linked one-to-one to a cluster (Option 4 in the associations proposal).

It is the responsibility of the ElasticsearchClusterController to watch that resource and apply the license to the cluster. 

Pros: 
* simple
* in principle orthogonal to option 2

Cons:
* user has to manage license themselves create/upload/update when it expires


```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: ClusterLicense
metadata:
  name: <cluster-name>
  namespace: <cluster-namespace>
spec:
  uid: "893361dc-9749-4997-93cb-802e3d7fa4xx" 
  type: "standard"
  issueDate: 1411948800000 # MicroTime?
  expiryDate: 1914278399999 # MicroTime?
  issuedTo: "issuedTo"
  issuer: "issuer"
  signatureRef: {
    name: <secret-name>
    key: <optional secret key for secrets containing multiple sigs> 
  }
```


### Option  2: License pool and license controller 

We support a pool of licenses and create a license controller that applies the most suitable license to the individual cluster deployments.  Pool of licenses is to be understood as one or more enterprise licenses in the namespace of the license controller. The controller selects a license from the pool of licenses using one of two heuristics:
1. Find a suitable license. Suitable is defined similar to our current practice in ESS: in descending precedence order of platinum, gold, standard and with the best match with regards to license validity (at least n days after license start, at least n days before license expiry). 
2. The cluster can also express the desire to be issued with a specific kind of license through a special label for example `k8s.elastic.co\desired-license=platinum` in which case the controller tries to find a match with regards to license validity (at least n days after license start, at least n days before license expiry) of the right type. 


#### Workflow
* user creates a combination of secrets with license signatures and corresponding ClusterLicense/EnterpriseLicense CRs in the namespace of the license controller
* the license controller watches all Elasticsearch clusters in other namespaces and periodically checks whether a new `ClusterLicense` needs to be attached to them
* the actual license application is handled by the Elasticsearch cluster controller who has the necessary credentials. No need to have credentials in the license controller


```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: EnterpriseLicense
metadata:
  name: <license-name>
  namespace: <license-controller-namespace>
spec:
  uid: "893361dc-9749-4997-93cb-802e3d7fa4xx" 
  type: "enterprise"
  issueDate: 1411948800000 # MicroTime?
  expiryDate: 1914278399999 # MicroTime?
  maxInstances: 40
  issuedTo: "issuedTo"
  issuer: "issuer"
  signatureRef: {
    name: <secret-name>
    key: <optional secret key for secrets containing multiple sigs> 
  }
  clusterLicenses: 
    - uid <cluster-license-spec-inline>
```


## Decision Outcome

Option 1 and 2 are both valid as two separate implementation phases. Without a running license controller option 1 still ensures licenses can be applied to Elasticsearch.

### Positive Consequences <!-- optional -->

* both options are orthogonal to each other
* when used in combination user just needs to create Enterprise licenses in the namespace of the license controller
* one license controller can manage licenses for all Elasticsearch clusters 

### Negative Consequences <!-- optional -->

* we are limited to Enterprise licenses for option 2 at the moment (could be revisited)


## Questions: 

* What kind of license will we support? gold, platinum, standard license?
    * assume all license types for now  
* What do we do when the license expires. How do we recover from that?
    * The cluster does not disintegrate when the license expires. The operator will detect the cluster as unhealthy as our current health checks start failing only if the cluster never had a license. But we might continue starting out with a trial license until the cluster forms and is issued a proper license.  The license API stays responsive even without any license attached to the cluster and cluster bounces back as soon as a valid license is put into place. 
* How do we handle license downgrades to basic? 
    * As basic does not support internal TLS, there is no way at the moment to downgrade to basic. Should we prevent/validate this with a CRD and validation?
* Do we have have way of testing licensing. Can we generate test licenses?
    * Not really. The only option is to unit/integration test the code around license management. We can think about two options in the future: dev licenses similar to what Elasticsearch does (would require us to use dev Docker images) or loading a valid license in vault for CI to run a set of integration and e2e tests on it
* Do we need to support enterprise licenses that contain individual cluster licenses?
    * Yes, in phase 2 the license pool shout support enterprise licenses
* Do we really need a custom resource definition for the license or could it just be a secret?
    * In theory a secret would suffice. We opted for a custom resource type with a secret reference to have a structured resource with known fields instead of a generic associative bag. A custom resource also allows validations if we want that.


