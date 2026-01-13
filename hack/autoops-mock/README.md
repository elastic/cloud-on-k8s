# Cloud Connected API Mock Server

A mock HTTP server for the Cloud Connected API endpoint `/api/v1/cloud-connected/clusters`.

## Usage

Run the mock server:

```bash
go run main.go
```

The server will start on port 8080 by default. You can change the port by setting the `PORT` environment variable:

```bash
PORT=3000 go run main.go
```

## Endpoints

### POST `/api/v1/cloud-connected/clusters`

Creates a Cloud Connected cluster.

**Query Parameters:**
- `create_api_key` (boolean, optional): If `true`, the response will include an API key.

**Request Body:**
```json
{
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
}
```

**Response:**
- `201 Created`: Returns the created cluster with full details
- `200 OK`: Returns existing cluster (if simulating existing cluster)
- `400 Bad Request`: Invalid request body
- `404 Not Found`: For any other endpoints

All requests are logged to stdout using standard HTTP log format.

## Features

- Uses only Go standard library (no external dependencies)
- Logs all HTTP requests with standard fields (remote_addr, method, path, status, size, etc.)
- Returns 404 for unmatched endpoints
- Supports the `create_api_key` query parameter
- Returns proper ETag headers
