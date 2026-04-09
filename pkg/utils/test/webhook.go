// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/webhook"
)

// ValidationWebhookTestCase represents a test case for testing a validation webhook
type ValidationWebhookTestCase struct {
	Name      string                                                      // Name of the test
	Operation admissionv1.Operation                                       // Operation type (Create, Update, or Delete)
	Object    func(t *testing.T, uid string) []byte                       // Object to check
	OldObject func(t *testing.T, uid string) []byte                       // Old object (for updates)
	Check     func(t *testing.T, response *admissionv1.AdmissionResponse) // Logic to check the response
}

// ValidationWebhookSucceeded is a helper function to verify that the validation webhook accepted the request.
func ValidationWebhookSucceeded(t *testing.T, response *admissionv1.AdmissionResponse) {
	t.Helper()
	require.True(t, response.Allowed, "Request denied: %s", response.Result.Reason)
}

// ValidationWebhookFailed is a helper function to verify that the validation webhook rejected the request.
func ValidationWebhookFailed(causeRegexes ...string) func(*testing.T, *admissionv1.AdmissionResponse) {
	return func(t *testing.T, response *admissionv1.AdmissionResponse) {
		t.Helper()
		require.False(t, response.Allowed)

		if len(causeRegexes) > 0 {
			require.NotNil(t, response.Result.Details, "Response must include failure details")
		}

		for _, cr := range causeRegexes {
			found := false
			t.Logf("Checking for existence of: %s", cr)
			for _, cause := range response.Result.Details.Causes {
				reason := fmt.Sprintf("%s: %s", cause.Field, cause.Message)
				t.Logf("Reason: %s", reason)
				match, err := regexp.MatchString(cr, reason)
				require.NoError(t, err, "Match '%s' returned error: %v", cr, err)
				if match {
					found = true
					break
				}
			}

			require.True(t, found, "[%s] is not present in cause list", cr)
		}
	}
}

// ValidationWebhookFailedWithWarnings returns a check function that asserts the admission request was denied,
// the denial causes match the provided causeRegexes, and the response warnings match the provided warningRegexes.
func ValidationWebhookFailedWithWarnings(causeRegexes []string, warningRegexes []string) func(*testing.T, *admissionv1.AdmissionResponse) {
	return func(t *testing.T, response *admissionv1.AdmissionResponse) {
		t.Helper()
		ValidationWebhookFailed(causeRegexes...)(t, response)
		require.Len(t, response.Warnings, len(warningRegexes), "unexpected number of warnings: %v", response.Warnings)
		for _, wr := range warningRegexes {
			found := false
			t.Logf("Checking for existence of warning: %s", wr)
			for _, warning := range response.Warnings {
				match, err := regexp.MatchString(wr, warning)
				require.NoError(t, err, "Match '%s' returned error: %v", wr, err)
				if match {
					found = true
					break
				}
			}
			require.True(t, found, "[%s] is not present in warning list", wr)
		}
	}
}

// ValidationWebhookSucceededWithWarnings returns a check function that asserts the admission request was
// allowed and that the response warnings match exactly the provided regexes (one regex per warning, in any order).
func ValidationWebhookSucceededWithWarnings(warningsRegexes ...string) func(*testing.T, *admissionv1.AdmissionResponse) {
	return func(t *testing.T, response *admissionv1.AdmissionResponse) {
		t.Helper()
		require.True(t, response.Allowed, "Request denied: %s", response.Result.Reason)
		require.Len(t, response.Warnings, len(warningsRegexes), "unexpected number of warnings: %v", response.Warnings)
		for _, wr := range warningsRegexes {
			found := false
			t.Logf("Checking for existence of: %s", wr)
			for _, warning := range response.Warnings {
				match, err := regexp.MatchString(wr, warning)
				require.NoError(t, err, "Match '%s' returned error: %v", wr, err)
				if match {
					found = true
					break
				}
			}
			require.True(t, found, "[%s] is not present in warning list", wr)
		}
	}
}

// NewValidationWebhookHandler creates an http.Handler for validation webhook tests
// using the upstream admission.Validator[T] interface.
func NewValidationWebhookHandler[T runtime.Object](validate webhook.ValidateFunc[T]) http.Handler {
	// Register the ECK types into the global scheme. Idempotent because of the use of once.Do.
	controllerscheme.SetupScheme()
	validator := webhook.NewResourceFuncValidator[T](license.MockLicenseChecker{}, nil, validate)
	return admission.WithValidator[T](clientgoscheme.Scheme, validator)
}

// RunValidationWebhookTests runs a series of ValidationWebhookTestCases against an http.Handler.
// The resource parameter should be the plural resource name (e.g., "agents", "apmservers").
//
//nolint:thelper
func RunValidationWebhookTests(t *testing.T, gvk metav1.GroupVersionKind, resource string, handler http.Handler, tests ...ValidationWebhookTestCase) {
	decoder := serializer.NewCodecFactory(clientgoscheme.Scheme).UniversalDeserializer()

	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()

	for _, tt := range tests {
		tc := tt
		t.Run(tc.Name, func(t *testing.T) {
			uid := tc.Name
			payload := &admissionv1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{Kind: "AdmissionReview"},
				Request: &admissionv1.AdmissionRequest{
					UID:       types.UID(uid),
					Kind:      gvk,
					Resource:  metav1.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: resource},
					Operation: tc.Operation,
					Object:    runtime.RawExtension{Raw: tc.Object(t, uid)},
				},
			}

			if tc.Operation == admissionv1.Update {
				payload.Request.OldObject = runtime.RawExtension{Raw: tc.OldObject(t, uid)}
			}

			payloadBytes, err := json.Marshal(payload)
			require.NoError(t, err)

			ctx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelFunc()

			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, bytes.NewReader(payloadBytes))
			require.NoError(t, err)

			request.Header.Add("Content-Type", "application/json")
			resp, err := client.Do(request)
			require.NoError(t, err)
			defer func() {
				if resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}()

			response := decodeResponse(t, decoder, resp.Body)
			tc.Check(t, response)
		})
	}
}

func decodeResponse(t *testing.T, decoder runtime.Decoder, body io.Reader) *admissionv1.AdmissionResponse {
	t.Helper()

	responseBytes, err := io.ReadAll(body)
	require.NoError(t, err, "Failed to read response body")

	response := &admissionv1.AdmissionReview{}
	_, _, err = decoder.Decode(responseBytes, nil, response)
	require.NoError(t, err, "Failed to decode response")

	return response.Response
}
