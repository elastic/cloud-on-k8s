// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	defaultPort = "8080"
	endpoint    = "/api/v1/cloud-connected/clusters"
)

// Default response JSON
const defaultResponse = `{
  "id": "iu0xjx9nz1uhjuvb08qn18qdqs4s0ga3",
  "name": "my observability cluster",
  "metadata": {
    "created_at": "%s",
    "created_by": "1014289666002276",
    "organization_id": "198583657190"
  },
  "self_managed_cluster": {
    "id": "string",
    "name": "observability-cluster-central-2",
    "version": "8.10.1"
  },
  "license": {
    "type": "trial",
    "uid": "1234567890abcdef1234567890abcdef"
  },
  "services": {
    "auto_ops": {
      "enabled": true,
      "support": {
        "supported": true,
        "valid_license_types": ["trial"],
        "minimum_stack_version": "8.5.0"
      },
      "config": {
        "region_id": "aws-us-east-1"
      },
      "metadata": {
        "documentation_url": "https://www.elastic.co/guide/en/cloud/current/eis.html",
        "service_url": "https://app.auto-ops.cloud.elastic.co/regions/aws-us-east-1/organizations/198583657190/clusters/iu0xjx9nz1uhjuvb08qn18qdqs4s0ga3/cluster",
        "connect_url": "https://application.auto-ops.cloud.elastic.co/organizations/198583657190/connect-autoops"
      },
      "subscription": {
        "required": true
      }
    },
    "eis": {
      "enabled": true,
      "support": {
        "supported": true,
        "valid_license_types": ["trial"],
        "minimum_stack_version": "8.5.0"
      },
      "metadata": {
        "documentation_url": "https://www.elastic.co/guide/en/cloud/current/eis.html"
      },
      "subscription": {
        "required": true
      }
    }
  },
  "key": "VXNlci1JRDoxMjM0NTY3ODkwYWJjZGVmMTIzNDU2Nzg5MGFiY2RlZg=="
}`

func createClusterHandler(w http.ResponseWriter, r *http.Request) {
	// Generate response with current timestamp
	now := time.Now().UTC().Format(time.RFC3339Nano)
	response := fmt.Sprintf(defaultResponse, now)

	// Set headers
	w.Header().Set("ETag", `"mock-etag-12345"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(response))
}

func respondWithError(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errorResp := fmt.Sprintf(`{"errors":[{"message":"%s","code":"%s"}]}`, message, code)
	w.Write([]byte(errorResp))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Log the request using standard HTTP log format
		duration := time.Since(start)
		log.Printf(
			"%s %s %s %s %d %d %q %q %q %.3f",
			r.RemoteAddr,
			r.Method,
			r.URL.Path,
			r.Proto,
			wrapped.statusCode,
			wrapped.size,
			r.Referer(),
			r.UserAgent(),
			r.URL.RawQuery,
			duration.Seconds(),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()

	// Handle the create cluster endpoint
	mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "method.not.allowed")
			return
		}
		createClusterHandler(w, r)
	})

	// Handle all other requests with 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found")
	})

	// Wrap with logging middleware
	handler := loggingMiddleware(mux)

	addr := ":" + port
	log.Printf("Starting mock Cloud Connected API server on %s", addr)
	log.Printf("Endpoint: POST %s", endpoint)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
