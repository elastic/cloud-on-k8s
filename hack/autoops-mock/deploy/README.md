# AutoOps Mock Server Helm Chart

This Helm chart deploys the AutoOps Mock Server, a mock HTTP server for the Cloud Connected API endpoint `/api/v1/cloud-connected/clusters`.

## Installation

### Prerequisites

- Kubernetes 1.21+
- Helm 3.0+

### Install the Chart

To install the chart with the release name `autoops-mock`:

```bash
helm install autoops-mock . --namespace autoops-mock --create-namespace
```

### Install with Custom Image

If you've built a custom Docker image:

```bash
helm install autoops-mock . \
  --namespace autoops-mock \
  --create-namespace \
  --set image.repository=your-registry/autoops-mock \
  --set image.tag=your-tag
```

### Uninstall the Chart

```bash
helm uninstall autoops-mock --namespace autoops-mock
```

## Configuration

The following table lists the configurable parameters and their default values:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Container image repository | `autoops-mock` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Container image tag | `""` (uses Chart.AppVersion) |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `service.targetPort` | Container port | `8080` |
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `50m` |
| `resources.requests.memory` | Memory request | `64Mi` |

## Usage

After installation, the mock server will be available at:

- **ClusterIP**: Accessible within the cluster at `http://autoops-mock:8080`
- **NodePort/LoadBalancer**: Accessible via the external IP/port

### Test the API

Port-forward to access the service locally:

```bash
kubectl port-forward svc/autoops-mock 8080:8080 -n autoops-mock
```

Then test the endpoint:

```bash
curl -X POST http://localhost:8080/api/v1/cloud-connected/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My observability cluster",
    "self_managed_cluster": {
      "id": "abcdefghi12345678",
      "name": "self-managed-cluster-id",
      "version": "8.5.3"
    },
    "license": {
      "type": "enterprise",
      "uid": "1234567890abcdef1234567890abcdef"
    }
  }'
```

### With API Key

To get an API key in the response:

```bash
curl -X POST "http://localhost:8080/api/v1/cloud-connected/clusters?create_api_key=true" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My observability cluster",
    "self_managed_cluster": {
      "id": "abcdefghi12345678",
      "name": "self-managed-cluster-id",
      "version": "8.5.3"
    },
    "license": {
      "type": "enterprise",
      "uid": "1234567890abcdef1234567890abcdef"
    }
  }'
```

## Resources

The chart creates the following Kubernetes resources:

- **Deployment**: Runs the mock server pods
- **Service**: Exposes the mock server within the cluster
- **ServiceAccount**: Service account for the pods (optional, created by default)
