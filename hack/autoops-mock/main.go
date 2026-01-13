// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package main

import (
	"encoding/json"
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

type SelfManagedCluster struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type License struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

type CreateClusterRequest struct {
	Name               string             `json:"name"`
	SelfManagedCluster SelfManagedCluster `json:"self_managed_cluster"`
	License            License            `json:"license"`
}

type Metadata struct {
	CreatedAt      string `json:"created_at"`
	CreatedBy      string `json:"created_by"`
	OrganizationID string `json:"organization_id"`
}

type ServiceSupport struct {
	Supported           bool     `json:"supported"`
	ValidLicenseTypes   []string `json:"valid_license_types"`
	MinimumStackVersion string   `json:"minimum_stack_version"`
}

type ServiceConfig struct {
	RegionID string `json:"region_id"`
}

type ServiceMetadata struct {
	DocumentationURL string `json:"documentation_url"`
	ServiceURL       string `json:"service_url,omitempty"`
	ConnectURL       string `json:"connect_url,omitempty"`
}

type ServiceSubscription struct {
	Required bool `json:"required"`
}

type Service struct {
	Enabled      bool                `json:"enabled"`
	Support      ServiceSupport      `json:"support"`
	Config       *ServiceConfig      `json:"config,omitempty"`
	Metadata     ServiceMetadata     `json:"metadata"`
	Subscription ServiceSubscription `json:"subscription"`
}

type Services struct {
	AutoOps Service `json:"auto_ops"`
	EIS     Service `json:"eis"`
}

type CreateClusterResponse struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Metadata           Metadata           `json:"metadata"`
	SelfManagedCluster SelfManagedCluster `json:"self_managed_cluster"`
	License            License            `json:"license"`
	Services           Services           `json:"services"`
	Key                string             `json:"key,omitempty"`
}

type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func generateClusterID() string {
	// Simple ID generation - in a real scenario this would be more sophisticated
	return "iu0xjx9nz1uhjuvb08qn18qdqs4s0ga3"
}

func generateAPIKey() string {
	// Base64 encoded example key
	return "VXNlci1JRDoxMjM0NTY3ODkwYWJjZGVmMTIzNDU2Nzg5MGFiY2RlZg=="
}

func createClusterHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	createAPIKey := false
	if r.URL.Query().Get("create_api_key") == "true" {
		createAPIKey = true
	}

	// Parse request body
	var req CreateClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", "invalid.request.body")
		return
	}

	// Generate response
	now := time.Now().UTC().Format(time.RFC3339Nano)
	response := CreateClusterResponse{
		ID:   generateClusterID(),
		Name: req.Name,
		Metadata: Metadata{
			CreatedAt:      now,
			CreatedBy:      "1014289666002276",
			OrganizationID: "198583657190",
		},
		SelfManagedCluster: req.SelfManagedCluster,
		License:            req.License,
		Services: Services{
			AutoOps: Service{
				Enabled: true,
				Support: ServiceSupport{
					Supported:           true,
					ValidLicenseTypes:   []string{"trial", "enterprise"},
					MinimumStackVersion: "8.5.0",
				},
				Config: &ServiceConfig{
					RegionID: "aws-us-east-1",
				},
				Metadata: ServiceMetadata{
					DocumentationURL: "https://www.elastic.co/guide/en/cloud/current/eis.html",
					ServiceURL:       fmt.Sprintf("https://app.auto-ops.cloud.elastic.co/regions/aws-us-east-1/organizations/198583657190/clusters/%s/cluster", generateClusterID()),
					ConnectURL:       "https://application.auto-ops.cloud.elastic.co/organizations/198583657190/connect-autoops",
				},
				Subscription: ServiceSubscription{
					Required: true,
				},
			},
			EIS: Service{
				Enabled: true,
				Support: ServiceSupport{
					Supported:           true,
					ValidLicenseTypes:   []string{"trial", "enterprise"},
					MinimumStackVersion: "8.5.0",
				},
				Metadata: ServiceMetadata{
					DocumentationURL: "https://www.elastic.co/guide/en/cloud/current/eis.html",
				},
				Subscription: ServiceSubscription{
					Required: true,
				},
			},
		},
	}

	// Add API key if requested
	if createAPIKey {
		response.Key = generateAPIKey()
	}

	// Set ETag header
	w.Header().Set("ETag", `"mock-etag-12345"`)

	// Return 201 for new cluster (or 200 if simulating existing)
	// Default to 201 for new clusters
	statusCode := http.StatusCreated
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func respondWithError(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errorResp := ErrorResponse{
		Errors: []ErrorDetail{
			{
				Message: message,
				Code:    code,
			},
		},
	}
	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
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
