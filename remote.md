# Remote Clusters in ECK

ECK lets one Elasticsearch cluster talk to another as a "remote cluster" (for cross-cluster search / cross-cluster replication). There are two supported connection modes, both driven from the same CR field, and two cooperating controllers implement the feature.

- **Legacy / certificate-based (transport) mode**: mutual CA trust between the transport layers (port 9300), no API key.
- **RCS2 / API-key mode** (ES ≥ 8.10.0): a dedicated "remote cluster server" listener (port 9443) plus a Cross-Cluster API key for authentication, on top of the same mutual CA trust.

Both modes require an Enterprise license.

## 1. Specifying a remote cluster in the CR

### Go types

`pkg/apis/elasticsearch/v1/elasticsearch_types.go`:

```go
type ElasticsearchSpec struct {
    ...
    ServiceAccountName  string                `json:"serviceAccountName,omitempty"`
    RemoteClusters      []RemoteCluster       `json:"remoteClusters,omitempty"`
    RemoteClusterServer RemoteClusterServer   `json:"remoteClusterServer,omitempty"`
    ...
}

// RemoteClusterServer controls whether this cluster exposes the RCS2 listener.
type RemoteClusterServer struct {
    Enabled bool                     `json:"enabled,omitempty"`
    Service commonv1.ServiceTemplate `json:"service,omitempty"`
}

// RemoteCluster declares a remote Elasticsearch cluster connection.
type RemoteCluster struct {
    Name             string                      `json:"name"`
    ElasticsearchRef commonv1.LocalObjectSelector `json:"elasticsearchRef,omitempty"`
    APIKey           *RemoteClusterAPIKey         `json:"apiKey,omitempty"`
}
```

`pkg/apis/elasticsearch/v1/remote_cluster.go`:

```go
type RemoteClusterAPIKey struct {
    Access RemoteClusterAccess `json:"access,omitempty"`
}

// Mirrors the body of ES's "Create Cross-Cluster API key" API.
type RemoteClusterAccess struct {
    Search      *Search      `json:"search,omitempty"`
    Replication *Replication `json:"replication,omitempty"`
}
type Search struct {
    Names                  []string         `json:"names,omitempty"`
    FieldSecurity          *FieldSecurity   `json:"field_security,omitempty"`
    Query                  *commonv1.Config `json:"query,omitempty"`
    AllowRestrictedIndices bool             `json:"allow_restricted_indices,omitempty"`
}
type Replication struct { Names []string `json:"names,omitempty"` }
```

Whether `APIKey` is set is the single switch that decides which of the two connection modes ECK sets up for that remote cluster entry.

Helper methods on `*Elasticsearch`:
- `SupportsRemoteClusterAPIKeys()` — compares `Status.Version` against `RemoteClusterAPIKeysMinVersion` (8.10.0); returns `nil` ("not decided yet") if the cluster hasn't reported a version.
- `HasRemoteClusterAPIKey()` — true if any entry has `APIKey != nil`.

### YAML shape

Client cluster (initiates the connection, references the remote):

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata: {name: cluster1, namespace: ns1}
spec:
  version: 9.4.2
  remoteClusters:
    - name: to-ns2-cluster2
      elasticsearchRef:
        name: cluster2
        namespace: ns2
      apiKey:                       # presence => RCS2 / API-key mode
        access:
          search:
            names: [kibana_sample_data_ecommerce]
  nodeSets: [...]
```

Server cluster (must expose the RCS listener when accessed via API key):

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata: {name: cluster2, namespace: ns2}
spec:
  version: 9.4.2
  remoteClusterServer:
    enabled: true
  nodeSets: [...]
```

If `apiKey` is omitted, ECK falls back to legacy/certificate mode: no cross-cluster API key is created, and the seed address points at the target's plain transport Service (port 9300) instead of the RCS Service (port 9443).

Sample recipe: `config/recipes/remoteclusters/elasticsearch.yaml`. CRD schema lives in `config/crds/v1/resources/elasticsearch.k8s.elastic.co_elasticsearches.yaml`; API reference in `docs/reference/api-reference/main.md` (`RemoteCluster`, `RemoteClusterAPIKey`, `RemoteClusterAccess`, `RemoteClusterServer`).

Validation:
- `pkg/controller/elasticsearch/validation/validations.go` — rejects `remoteClusterServer.enabled` or any `remoteClusters[].apiKey` when `spec.version < 8.10.0`.
- `pkg/controller/elasticsearch/validation/stateless_validation.go` — `remoteClusters`/`remoteClusterServer.enabled` are currently forbidden entirely for stateless Elasticsearch.

## 2. Two controllers reconcile remote clusters

| Controller | Registered as | Package | Responsibility |
|---|---|---|---|
| Elasticsearch main controller | `Elasticsearch` (`cmd/manager/main.go`) | `pkg/controller/elasticsearch/remotecluster` | Pushes `cluster.remote.*` persistent cluster settings (the seed addresses) into the target ES cluster via the ES REST API, as part of the normal per-cluster reconcile. |
| RemoteCA controller | `RemoteCA` (`cmd/manager/main.go`) | `pkg/controller/remotecluster` | Standalone, cross-namespace controller: copies transport CA certs between the two clusters (mutual trust) and manages Cross-Cluster API keys + the keystore Secret that carries them. |

### 2a. `pkg/controller/remotecluster` — CA trust + API keys

#### What triggers reconciliation (`watches.go`)

1. Watch on `Elasticsearch` objects directly — any change requeues itself.
2. Watch on `Elasticsearch` objects mapped so that a change to cluster A's `Spec.RemoteClusters[i].ElasticsearchRef` enqueues a reconcile for the *referenced* cluster B. This is how a client's spec change triggers reconciliation of the server it references.
3. Watch on `Secret` objects, mapped via label matching:
   - Secrets labeled as a copied remote-CA (type label + `RemoteClusterNamespaceLabelName`/`RemoteClusterNameLabelName`) requeue the referenced remote cluster.
   - Secrets labeled as the remote-cluster API-keys keystore requeue the owning cluster.
4. A dynamic per-relationship watch on each peer's public transport-CA Secret, added/removed as relationships are created/destroyed, so CA rotation on either side retriggers reconciliation.
5. `watches.WatchNamespaceFlips(...)` for multi-tenant namespace-selector changes.

#### `Reconcile` (`controller.go`)

The request names the cluster acting as **remote server** for this pass (i.e., whose incoming relationships are being reconciled).

1. Namespace-scope check; if out of scope, forget any cached keystore state for that cluster.
2. Fetch the `Elasticsearch` object.
3. If not found: delete all CA Secrets (both directions) for every cluster that had a relationship with it, and remove the associated dynamic watches.
4. If unmanaged: skip.
5. Otherwise, `doReconcile`:
   1. **Discover expected relationships**: list all `Elasticsearch` objects, build the set of clients whose `RemoteClusters[i].ElasticsearchRef` points at this server (also seed the map for anything this cluster itself references, in case it's acting as a client too).
   2. **License gate**: if Enterprise features are disabled and at least one relationship is expected, log and stop — no requeue.
   3. **Snapshot currently-associated CAs** (both directions) to garbage-collect stale trust relationships later.
   4. **If this cluster supports RCS2**: wait until it's reachable, build a real ES client, and fetch all existing `eck-*` Cross-Cluster API keys from it.
   5. **For each expected client cluster**:
      - If the client is gone, invalidate/garbage-collect its keys and move on.
      - **Bidirectional RBAC check**: does the server's `ServiceAccountName` have access to the client, and vice versa? If not, drop the relationship, emit a warning event, invalidate any existing keys for that client.
      - **CA sync** (unconditional, needed for both modes): copy each side's transport CA into the other's namespace.
      - **Version check**: both sides must have reported a status version before deciding RCS2 support; skip if the client is below the minimum version while the server is above it.
      - **`reconcileAPIKeys`**: create/update/invalidate the actual Cross-Cluster API key and keystore entry for this client (RCS2 mode only — see below).
   6. **Garbage collection**: invalidate any active `eck-*` API key in this cluster whose client wasn't touched this pass (skipping keys managed by AutoOps), and remove unexpected aliases from the local keystore Secret.
   7. **Delete orphaned CA Secrets** left over from the earlier snapshot.
   8. Result includes a periodic 15-minute requeue when RBAC is enforced via subject-access-review, since role/rolebinding changes aren't watched directly.

#### CA / trust logic (`secret.go`)

- `createOrUpdateCertificateAuthorities(local, remote)`: registers the dynamic watches, then copies CA both ways.
- `copyCertificateAuthority(source, target)`: reads the source's public transport-CA Secret; if missing, emits a warning event and relies on the watch to retrigger. Otherwise writes an owned Secret on the target named `<target>-<source.ns>-<source.name>-remote-ca`.
- `deleteCertificateAuthorities`: deletes both directions' copied-CA Secrets and their dynamic watches.

These per-peer CA Secrets are aggregated separately by `pkg/controller/elasticsearch/certificates/remoteca` (invoked from the main ES certs reconciliation): it lists all Secrets labeled `remote-ca` for a cluster and concatenates their CA certs (falling back to the cluster's own transport CA) into a single Secret. That aggregate is mounted into ES pods and referenced by `xpack.security.transport.ssl.certificate_authorities` and, when RCS is enabled, `xpack.security.remote_cluster_server.ssl.certificate_authorities`. Each node ends up trusting its own transport CA plus every peer CA it has a relationship with.

#### API key reconciliation (`apikey.go`, RCS2 path only)

For each client's `remoteClusterRef` pointing at this server:
- Deterministic key name: `eck-<clientNamespace>-<clientName>-<aliasName>`.
- If `APIKey` was removed from the spec but a key still exists, invalidate it.
- Otherwise compare a hash of the desired `RemoteClusterAccess` against a `config-hash` metadata tag on the live key:
  - Create it if missing (`CreateCrossClusterAPIKey`), recording the key ID and encoded value in the client's keystore.
  - Update it if the hash changed (`UpdateCrossClusterAPIKey`).
  - If the keystore's recorded key ID doesn't match what's live in ES, treat it as inconsistent: invalidate and force a resync.
- Metadata on every key created this way (`elasticsearch.k8s.elastic.co/{config-hash,name,namespace,uid,managed-by}`) lets the controller map a live ES key back to the owning Kubernetes client resource without a separate bookkeeping store.

The keystore itself (`pkg/controller/remotecluster/keystore`) is a Secret per client cluster (`<client>-es-remote-api-keys`-style name):
- Data keys of the form `cluster.remote.<alias>.credentials` — this is literally the ES secure-settings name, so the Secret can be consumed directly as a keystore source by the ES driver.
- An annotation holds a JSON map from alias to `{namespace, name, id}`.
- A `Provider`/`pendingChanges` layer smooths over read-after-write lag between writing the Secret and it showing up in the client cache.

#### RBAC gate (`rbac.go`)

`isRemoteClusterAssociationAllowed` requires both directions to be allowed (local→remote and remote→local `ServiceAccountName` access review), because a relationship implies mutual trust — CA exchange plus, in RCS2 mode, issuing an API key server-side on behalf of the client.

### 2b. `pkg/controller/elasticsearch/remotecluster` — pushing seeds into ES

Invoked in-band from the ES driver's reconcile once the cluster is reachable:

```go
requeue, err := remotecluster.UpdateSettings(ctx, client, esClient, params.Recorder, params.LicenseChecker, es)
```

- No-op unless `Spec.RemoteClusters` is non-empty or a tracking annotation already exists; license-gated on Enterprise features.
- Diffs three sources of truth: the spec, a tracking annotation (`elasticsearch.k8s.elastic.co/managed-remote-clusters`, comma-joined alias list) recording what ECK previously created, and the live ES persistent settings (`GET _cluster/settings`, `cluster.remote.*`).
- Updates the annotation first (so a crash mid-way is recoverable), then issues a single `PUT _cluster/settings`.
- **Seed host** is computed purely from Kubernetes Service DNS, no dynamic peer discovery:
  - Legacy mode (`APIKey == nil`): `<es>-es-transport.<ns>.svc:9300`.
  - RCS2 mode (`APIKey != nil`): `<es>-es-remote-cluster.<ns>.svc:9443`.
- Removal is represented as `RemoteCluster{Seeds: nil}`, which deletes the remote cluster definition in ES.

## 3. Supporting infrastructure

- **Services** (`pkg/controller/elasticsearch/services/services.go`): `NewTransportService` (headless, port 9300, always present) and `NewRemoteClusterService` (headless, port 9443, only created when `remoteClusterServer.enabled: true`).
- **TLS config** (`pkg/controller/elasticsearch/settings/merged_config.go`): when RCS is enabled, sets `xpack.security.remote_cluster_server.ssl.{key,certificate,certificate_authorities}` using the transport cert/key plus the aggregated remote-CA bundle; when acting as an RCS2 client, sets the equivalent `xpack.security.remote_cluster_client.ssl.*`.
- **ES REST client layer** (`pkg/controller/elasticsearch/client/remote_cluster.go`): wraps `PUT/GET _cluster/settings` for seeds, and the Cross-Cluster API Key security endpoints (`_security/cross_cluster/api_key`) for create/update/invalidate/list.

### Interaction with other controllers

- **License controller**: both remote-cluster controllers are gated on `EnterpriseFeaturesEnabled` — remote clusters (either mode) are Enterprise-only.
- **AutoOps controller**: the RemoteCA controller's garbage collection explicitly skips API keys tagged as managed by AutoOps, since AutoOps creates its own cross-cluster-style keys independently.
- **Association controller**: RemoteCA reuses `association.RequeueRbacCheck` for the periodic RBAC re-check, matching the pattern used by other cross-namespace associations (Kibana-ES, APM-ES, etc.).
- **`certificates/remoteca`**: aggregates the per-peer CA Secrets written by RemoteCA into the single bundle mounted into ES pods.
- **ES driver keystore/secure-settings**: consumes the per-client `*-es-remote-api-keys` Secret as a secure-settings source so the encoded API key ends up loaded into the client cluster's Elasticsearch keystore.

## 4. End-to-end flow (RCS2 / API-key mode)

1. User sets `spec.remoteClusterServer.enabled: true` on cluster **B** (server), and adds `spec.remoteClusters[]` with `apiKey: {...}` on cluster **A** (client), referencing B via `elasticsearchRef`.
2. The main ES controller reconciling B creates the `<B>-es-remote-cluster` Service on port 9443 and configures `xpack.security.remote_cluster_server.ssl.*`.
3. The RemoteCA controller, reconciling B (triggered because A references B), finds A as an expected client, does the bidirectional RBAC check, copies B's CA into A's namespace and A's CA into B's namespace, creates a Cross-Cluster API key in B scoped per `remoteClusterRef.APIKey.Access`, tagged with metadata identifying A, and writes the encoded key into A's `<A>-es-remote-api-keys` Secret as `cluster.remote.<alias>.credentials`.
4. `remoteca.Reconcile` aggregates the per-peer CA secrets on both A and B into the mounted CA bundle used by transport/RCS TLS.
5. The ES driver's keystore reconciliation loads A's remote-api-keys Secret as a secure-settings source, so A's nodes get the credential in their ES keystore.
6. The main ES controller reconciling A (`elasticsearch/remotecluster.UpdateSettings`) pushes `cluster.remote.<alias>.seeds = ["<B>-es-remote-cluster.<ns-B>.svc:9443"]` via `PUT _cluster/settings`, tracked by the `managed-remote-clusters` annotation on A.
7. A's Elasticsearch connects to B's remote-cluster-server endpoint, authenticating with the stored Cross-Cluster API key over mutually-trusted TLS.

Legacy/certificate mode skips the API-key steps (3's key creation, 5) and step 6 targets B's transport Service on port 9300 instead; mutual CA trust (steps 2-4, transport-only) is still required and is the sole trust mechanism.
