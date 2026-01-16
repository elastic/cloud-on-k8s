# WireMock Cloud Connected API Mock

A Helm chart that deploys a WireMock instance to mock the Elastic Cloud Connected User API for testing purposes.

## Overview

This chart provides a mock implementation of the Cloud Connected User API's create cluster endpoint. It's useful for:
- Local development and testing
- E2E tests without external dependencies
- Demo environments

## Supported Endpoint

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/cloud-connected/clusters` | Create a cloud-connected cluster |

## Installation

```bash
# From the cloud-on-k8s repository root
helm install cloud-connected-mock test/e2e/charts/wiremock-cloud-connected-api -n <namespace>
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | WireMock image repository | `wiremock/wiremock` |
| `image.tag` | WireMock image tag | `3.3.1` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `replicaCount` | Number of replicas | `1` |
| `mockConfig.defaultOrganizationId` | Organization ID in responses | `198583657190` |
| `mockConfig.defaultUserId` | User ID in responses | `1014289666002276` |
| `mockConfig.defaultRegion` | Default region | `aws-us-east-1` |

## Usage Example

```bash
# Port forward to access the mock service
kubectl port-forward svc/cloud-connected-mock-wiremock-cloud-connected-api 8080:8080

# Create a cluster
curl -X POST http://localhost:8080/api/v1/cloud-connected/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Cloud Connected Cluster",
    "self_managed_cluster": {
      "id": "my-cluster-id",
      "name": "my-cluster",
      "version": "8.12.0"
    },
    "license": {
      "type": "enterprise",
      "uid": "license-uid-123"
    }
  }'
```

## Response

The mock returns a response with:
- Randomly generated cluster ID
- Request body values echoed back
- Mock service configuration (AutoOps, EIS)
- Generated API key
