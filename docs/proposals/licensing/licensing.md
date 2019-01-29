## Design: Licensing

Proposal state: RFC

### Summary 
Purpose of this proposal is to outline an implementation for license management for Elasticsearch clusters managed by the Elastic k8s operator.

### Constraints 

* gold/platinum level licenses can only be applied to clusters using internal TLS
* user applying the license needs to have `manage` privileges if security features are enabled (which is always the case) 


### Phase 1: Directly applied license

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
    namespace: <secret-namespace>
    name: <secret-name>
    key: <optional secret key for secrets containing multiple sigs> 
  }
```


### Phase  2: License pool and license controller 

We support a pool of licenses and create a license controller that applies the most suitable license to the individual cluster deployments.  It selects a license from the pool of licenses using one of two heuristics:
* Suitable is defined similar to our current practice in ESS: in descending precedence order of platinum, gold, standard and with the best match with regards to license validity (at least n days after license start, at least n days before license expiry). 
* the cluster can express the desire to be issued with a specific kind of license via a special label for example `k8s.elastic.co\desired-license=platinum` in which case the controller tries to find a match with regards to license validity (at least n days after license start, at least n days before license expiry) of the right type. 

#### For all implementation options: 
* the controller watches all Elasticsearch clusters in its namespace and periodically checks/creates/links a `ClusterLicense` to them
* the actual license application is handled by the Elasticsearch cluster controller who has the necessary credentials. No need to have credentials in the license controller

#### Option 1
* user creates secrets with license data and marks the secrets with a label as Elasticsearch licenses `k8s.elastic.co\kind=license` 
* a license controller watches the set of secrets marked via label
* it then creates `ClusterLicense` CRs for each cluster

Pros: 
* No need to duplicate `ClusterLicense` to achieve a one-to-one relationship to a cluster

Cons:
* no validation options
* need to either mutate user created secret to create signature references or maintain and sync secrets with signature references based on the user created secrets
* unability to express nested (enterprise) licenses in the one level associative structure of a secret

#### Option 2
* a license controller manages a set of `ClusterLicense` CRs that it then links to individual cluster by creating copies with the right name or by attaching a label to them

Pros: 
* Same resource type used in pool and in cluster 
* No need to mutate/sync user created secret to license CRs
* Easy to watch from ES controller

Cons: 
* license controller has to watch and sync any changes from pool licenses to linked licenses


#### Option 3
* user creates licenses in pool consisting of a `ClusterLicense` CR and linked signature secret
* a license controller links those to individual clusters using a seperate `LicenseAssociation` CRD

Pros: 
* no duplication of secrets or license resources
* association CRD easy to list/discover from both sides--ES controller and license controller--given that they are labeled correctly

Cons: 
* potential inconsistencies when multiple associations are created per ES controller (which one should the ES controller apply?)
* ES controller needs two watches one for the association resource and one for the linked license 

#### Recommendations
Option 3 seems to have the least downsides for both ES controller and license controller. Ownership for the CRDs is with the license controller, the error case of multiple linked licenses can be handled in the ES controller (reapply the same heuristic as outlined above. 


### Questions: 

* What kind of license will we support? gold, platinum, standard license?
    * assume all license types for now  
* What do we do when the license expires. How do we recover from that?
    * The cluster does not disintegrate when the license expires. The operator will see the cluster as unhealthy as our current health checks start failing only if the cluster never had a license. But we might continue starting out with a trial license until the cluster forms and is issued a proper license.  The license API stays responsive even without any license attached to the cluster and cluster bounces back as soon as a valid license is put into place. 
* How do we handle license downgrades to basic? 
    * As basic does not support internal TLS I don't see a way at the moment to downgrade to basic. Should we prevent/validate this with a CRD and validation?
* Do we have have way of testing licensing. Can we generate test licenses?
    * Not really. The only option is to unit/integration test the code around license management. 
* Do we need to support enterprise licenses that contain individual cluster licenses?
    * Yes, in phase 2 the license pool shout support enterprise licenses
* Do we really need a custom resource definition for the license or could it just be a secret?
    * In theory a secret would suffice. We opted for a custom resource type with a secret reference to have a structured resource with known fields instead of a generic associative bag. A custom resource also allows validations if we want that.
