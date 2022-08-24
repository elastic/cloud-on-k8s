// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// APIError is a non 2xx response from the Elasticsearch API
type APIError struct {
	Status        string
	StatusCode    int
	ErrorResponse ErrorResponse
}

// newAPIError converts an HTTP response into an API error, attempting to parse the body to include the details about the error.
func newAPIError(ctx context.Context, response *http.Response) error {
	defer response.Body.Close()
	log := ulog.FromContext(ctx)
	apiError := &APIError{
		Status:     response.Status,
		StatusCode: response.StatusCode,
	}
	// We may need to read the body multiple times, read the full body and store it as an array of bytes.
	body, err := io.ReadAll(response.Body)
	if err != nil {
		// We were not able to read the body, log this I/O error and return the API error with the status.
		log.Error(err, "Cannot read Elasticsearch error response body")
		return apiError
	}
	// Reset the response body to the original unread state. It allows a caller to read again the body if necessary,
	// for example in the case of a 408.
	response.Body = io.NopCloser(bytes.NewBuffer(body))

	// Parse the body to get the details about the API error, as they are stored by Elasticsearch.
	var errorResponse ErrorResponse
	if err := json.Unmarshal(body, &errorResponse); err != nil {
		// Only log at the debug level since it is expected to not be able to parse all types of errors.
		// Some errors, like 408 on /_cluster/health may return a different body structure.
		log.V(1).Info("Unexpected Elasticsearch error response", "http.response.body.content", string(body))
		return apiError
	}
	apiError.ErrorResponse = errorResponse
	return apiError
}

// Error implements the error interface.
func (a *APIError) Error() string {
	return fmt.Sprintf("%s: %+v", a.Status, a.ErrorResponse)
}

// IsUnauthorized checks whether the error was an HTTP 401 error.
func IsUnauthorized(err error) bool {
	return isHTTPError(err, http.StatusUnauthorized)
}

// IsForbidden checks whether the error was an HTTP 403 error.
func IsForbidden(err error) bool {
	return isHTTPError(err, http.StatusForbidden)
}

// IsNotFound checks whether the error was an HTTP 404 error.
func IsNotFound(err error) bool {
	return isHTTPError(err, http.StatusNotFound)
}

// IsTimeout checks whether the error was an HTTP 408 error
func IsTimeout(err error) bool {
	return isHTTPError(err, http.StatusRequestTimeout)
}

// IsConflict checks whether the error was an HTTP 409 error.
func IsConflict(err error) bool {
	return isHTTPError(err, http.StatusConflict)
}

func Is4xx(err error) bool {
	apiErr := new(APIError)
	if errors.As(err, &apiErr) {
		code := apiErr.StatusCode
		return code >= 400 && code <= 499
	}
	return false
}

func isHTTPError(err error, statusCode int) bool {
	apiErr := new(APIError)
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == statusCode
	}
	return false
}

func newDecoratedHTTPError(request *http.Request, err error) error {
	if request == nil {
		return err
	}
	return fmt.Errorf(`elasticsearch client failed for %s: %w`, request.URL.Redacted(), err)
}
