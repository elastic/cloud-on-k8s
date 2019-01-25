## Design: Licensing

Proposal state: RFC

### Summary 
Purpose of this proposal is to outline an implementation for license management for Elasticsearch clusters managed by the Elastic k8s operator.

### Constraints 

* gold/platinum level licenses can only be applied to clusters using internal TLS
* user applying the license needs to have `manage` privileges if security features are enabled (which is always the case) 


### Option/Phase 1: Directly applied license

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
  signature: ""
```


### Option/Phase  2: License pool and license controller 

We support a pool of licenses and create a license controller that applies the most suitable license to the individual cluster deployments. Suitable is defined similar to our current practice in Cloud. It selects a license from the pool of licenses in descending precedence order of platinum, gold, standard and with the best match with regards to license validity (at least n days after license start, at least n days before license expiry). 

* a license controller manages a set of secrets marked via label as Elasticsearch licenses `k8s.elastic.co\kind=license`

* it watches all Elasticsearch clusters in its namespace and periodically creates a `ClusterLicense` 
* the actual license application is handled by the Elasticsearch cluster controller who has the necessary credentials 

Pros: 
* no need to expose ES user data to the license controller

Cons: 
* little control over what kind of license is applied to the cluster beyond the heuristic outlined above


### Questions: 

* What kind of license will we support? gold, platinum, standard license?
* For option 2 should we support an annotation on the cluster to disable automatic license management? 
* What do we do when the license expires. How do we recover from that?
  * The cluster does not disintegrate when the license expires. The operator will see the cluster as unhealthy as our current health checks start failing (do we need to handle this condition?) The license API stays responsive and cluster bounces back as soon as a valid license is put into place. 
* How do we handle license downgrades to basic? 
  * As basic does not support internal TLS I don't see a way at the moment to downgrade to basic. Should we prevent/validate this with a CRD and validation?
* Do we have have way of testing licensing. Can we generate test licenses?
* Do we need to support enterprise licenses that contain individual cluster licenses?
* Do we really need a custom resource definition for the license or could it just be a secret?
