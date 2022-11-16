# Elasticsearch nodes certificate management

**Update (2019-07-30):**
We decided to simplify the way we handle self-signed TLS certificates and nodes private keys. They are still managed by the operator, but signed by the operator then passed to ES pods through secrets directly. The main benefit is to remove the complexity of the CSR init container outlined in this proposal. We now also handle a set of certificates for the Transport protocol, and another set for the HTTP protocol. User-provided certificates are also supported.

* Status: proposed
* Deciders: k8s team
* Date: 2019-02-22

## Context and Problem Statement

Any production-grade Elasticsearch setup must be set up with TLS certificates. It is even more important in a Kubernetes cluster, where any running service could misbehave and impersonate an Elasticsearch node.

How do we manage nodes certificates with our operator?

## Decision Drivers

* Security
  * Each node should have its own private key, signed by a CA.
  * Private keys for both nodes and CA should be stored in such a way they very hard to obtain and/or decipher.
  * Certificate rotation should be easily doable by design.
  * ES pods should not access the Kubernetes API directly.
* Ease of use
  * Human operator intervention in running the operator regarding certificate management should be restricted as a minimum (ideally, the human operator should not care at all).
* Cluster impact
  * Certificate rotation should not provoke any downtime on the cluster.
  * Certificate rotation should preferably be doable without well-written clients experiencing issues with certificate validation.

## Considered Options

### Option 1 - operator retrieves CSRs from the pod and issues certificates through a secret volume mount

#### CA

The operator on the cluster *is* the certificate authority (CA) for all the clusters it manages. It is able to issue certificates based on certificate signing requests (CSR) that pods provide. It manages one CA certificate (and associated private key) per cluster (for ex. if the operator manages 3 clusters in 3 different namespaces, it uses 3 different CAs).

#### Issuing certificates

At each reconciliation loop, the operator can decide to issue a signed certificate for a given pod. It makes sense to issue a new certificate when:

* there is no certificate (for ex. the pod was just created)
* existing certificate is expired
* existing certificate is invalid or corrupted
* existing certificate does not match the current CA
* we wish to rotate the certificates

The certificate is created as a secret in the apiserver, and mounted to the pod through a secret volume mount. On the pod, Elasticsearch is able to use the provided certificate file. If the secret is updated, Elasticsearch will use the updated file on disk automatically (which is a pretty great feature). Unfortunately most software don't work the same way, and require a restart to pick up new certificates. Kibana for example does require a restart.

#### CA private key

The CA private key for each cluster is the most important secret in the whole mechanism. It must be well hidden, and should be accessible from the operator only (whereas the CA certificate is not sensitive).

Some options:

* *Option A*: the private key is generated when the operator starts on first cluster reconciliation, and kept in-memory for the lifecycle of the operator. If the operator restarts, it needs to generate a new private key, and issues new certificates for all clusters. While existing TCP connections between nodes will not be impacted, this might provoke a downtime on the cluster while new certificates are propagated.
* *Option B*: the private key is stored as a secret in the apiserver. Created by the operator itself if it does not exist yet. Persisted and reuse through operator restarts.
* *Option C*: similar to B, but with the private key [wrapped in another layer of encryption](https://tools.ietf.org/html/rfc3394) in the apiserver. Access to the KEK (key encryption-key) must be handled separately.
* *Option D*: the private key is stored in an external service (for ex. Vault) and requested or provided to the operator at startup.

Options C and D are preferable, but need to setup an additional system. Option A is secure, but can lead to downtime and a herd effect: when the operator restarts, it needs to regenerate all certificates. Option B is not secure if we consider the apiserver storage as not secure enough.

#### Pod private keys

Each pod must have its own private key. Several options considered here:

* *Option A*: the operator generates a private key for the pod, stores it in an apiserver secret, and mounts it to the pod through a secret volume. Major drawback: all pods private keys are stored in a single place (the apiserver). The operator can generate a CSR and issue a certificate from this private key.
* *Option B*: pods are responsible to generate their own private key and CSR on startup, in an init container. The CSR is retrieved by the operator to issue a certificate. The private key never leaves the Elasticsearch pod. It is only shared between the init container and the Elasticsearch pod.

Option B is the chosen one here. It is more complex to implement, but is more secure.

#### Retrieving pods CSRs

Considering the Elasticsearch pod created a private key and CSR on startup, the operator must be able to retrieve the CSR in order to issue a certificate.
CSR is not a secret information and can go public, as long as the private key stays private to the pod.

Several options considered involving an init container in the ES pod:

* *Option A*: the init container stores the CSR in the apiserver for the operator to pick up. Similar to how it's done [to bootstrap Kubernetes nodes](https://Kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/). There is already a built-in `CertificateSigningRequest` resource available. Advantage: it uses built-in mechanisms, the operator just has to watch CSR resources in order to issue new certificates. Drawback: the pod must be able to reach the apiserver, with a service account having the appropriate permissions. So far, this was not required. Considering any custom plugin could run in the Elasticsearch process, allowing requests to the apiserver might represent a security issue.
* *Option B*: the init container requests the operator through an API in the operator to send the CSR. Disadvantage: similar to Option A, we implicitly authorize the ES pod to reach the operator. Even though this can be restricted to a single endpoint, it's an additional possible flow that could lead to security issues.
* *Option C*: the init container runs an HTTP server to serve the generated CSR. The operator requests the CSR through this API. Advantage: the pod does not need to reach any other service. Disadvantage: some additional complexity in the design and the implementation.

Option C is the chosen one here. For more details on the actual workflow, check the [cert-initializer README](https://github.com/elastic/cloud-on-k8s/blob/main/operators/cmd/cert-initializer/README.md).

#### Pods certificate rotation

It is possible to reuse a previous CSR to issue a new certificate. In order to do so, there must be a way to retrieve pods CSRs.

* *Option A*: when retrieved the first time, store the CSR in the apiserver (can be in the same secret as the certificate itself). They can then just be retrieved from the apiserver.
* *Option B*: request the ES pod again for the CSR. Requires an endpoint to be available as a sidecar container, and not only as an init container.

Chosen option: A, simpler to implement.

#### Pods private key rotation

We might be easily able to rotate a pod certificate by reusing the same CSR (or a different one generated from the same private key). However, a pod private key might get compromised.

Options to consider:

* *Option A*: keep a sidecar container running in ES pod to handle private key rotation.
  * When notified by the operator that it should rotate its private key, make it create a new private key, create a CSR from this new private key, wait for the new certificate to be issued to a temporary place, then replace both the old private key and certificate by the new private key and certificate.
  * The sidecar could be notified by the operator either through an HTTP request, or a special file in a mounted secret volume.
  * If done on a regular basis, the sidecar itself could be responsible for the decision of rotating its private key on its own, and does not even need a notification from the operator. The operator still needs to know there is a new CSR to request.
* *Option B*: don't rotate private keys in pods.
  * If a pod private key is compromised, replace the pod by a new one, for which a new private key, CSR and certificate will be created.
  * It requires some extra mechanism in the operator to safely evacuate or replace a pod by a new one.
  * It could take some time (compared to option A) to safely migrate data.

Chosen option: option B. Simpler to implement, and we do need a way to safely replace/evacuate pods for other reasons (For example, node going down, hdd failures, and so on).

#### CA cert rotation

The operator should be able to rotate the CA certificate of a cluster for multiple reasons:

* it will expire soon
* CA private key has been compromised
* on a regular basis for security reasons

When rotating the CA cert, all nodes having certificates signed by the CA are impacted. New certificates need to be issued for those nodes. If the operator can access the nodes CSRs, it can reissue valid certificates using those CSR.

#### CA private key rotation

The operator should be able to rotate the CA private key of a cluster for multiple reasons:

* CA private key has been compromised
* on a regular basis for security reasons

Similar to the CA cert rotation section before, new certificates need to be issued for all nodes. In most cases, it makes sense to rotate both the CA certificate and the CA private key.

##### Avoiding downtime during rotation

While the CA cert is rotated and all nodes certificates are reissued, there can be an impact on connections. Long-running ES nodes TLS connections will be maintained (past the TLS handshake). New connections will likely fail until things get stabilized, incurring potential downtime.

Some options to handle that:

* *Option A: combined CA certs*
  * When rotating the CA, create a combined CA with both the old and the new CA
  * Update the CA certificate secret mounted on all nodes to contain the combined CA instead: nodes will accept connections from certificates signed by both the new and the old CA
  * Issue certificates to all nodes using the new CA
  * Once all nodes use their new certificate, update the CA certificate secret mounted on all nodes to contain only the new CA
* *Option B: cross-signed CA*
  * When rotating the CA, create a new intermediate CA, and cross-sign it with the old CA (needs the old CA private key to be available).
  * Same steps as option A.

Biggest advantage of Option B compared to Option A is that it is safe for a new pod to be created with a certificate issued from the new CA, where other pods in the cluster might still be using the old CA and don't have the new CA yet. It is a bit more complex to implement though.

### Option 2 - rely on a service mesh TLS implementation

Some service meshes handle TLS authentication automatically through sidecar containers mounted in pods (for ex. Istio and Envoy, Linkerd). Currently, it's mandatory in Elasticsearch to enable TLS encryption. Hence we cannot easily delegate TLS entirely to additional services. Moreover, it sounds wrong to force users to use a particular service mesh, that includes its own security problems.

### Option 3 - let users provide their own private keys and certificates

Instead of having the operator manage CAs and issue certificates signed by the CA, we could simply propagate existing CA and signed certificates from the user. The operator would mount secrets containing them to ES pods.

The user would store certificates and private keys as secrets in Kubernetes, and reference those secrets in the Elasticsearch cluster spec.

Benefits:

* Allows user to provide their own certificates (signed by their own CA, or by a well-known public CA)
* A lot simpler to implement in the operator

Drawbacks:

* Much more setup required from the user

## Decision Outcome

We decided to go with Option 1 (manage certificates in the operator). Major drawback is the additional complexity in the operator (and in init/sidecar containers).

Also, reaching the Elasticsearch cluster from any other service in the k8s cluster requires the CA cert to be available from that service. Which can be complex to handle, especially considering CA certificate rotation. This could be mitigated by adding an extra ingress proxy layer in front of Elasticsearch which would have its own certificate (potentially signed from a public well-known CA). Then forwarding requests to the ES cluster, using a second TLS handshake.

Even though choosing option 1 by default, option 3 could also be handled through configuration options to use user-provided certificates.

## Links

* [cert-initializer README](https://github.com/elastic/cloud-on-k8s/blob/main/operators/cmd/cert-initializer/README.md) describing interactions between the operator and the cert-initializer init container.
* [coordination with Kibana](https://github.com/elastic/cloud-on-k8s/issues/118)
