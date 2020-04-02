// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/stretchr/testify/require"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var log = logf.Log.WithName("test-webhook")

// ValidationWebhookTestCase represents a test case for testing a validation webhook
type ValidationWebhookTestCase struct {
	Name      string                                                           // Name of the test
	Operation admissionv1beta1.Operation                                       // Operation type (Create, Update, or Delete)
	Object    func(t *testing.T, uid string) []byte                            // Object to check
	OldObject func(t *testing.T, uid string) []byte                            // Old object (for updates)
	Check     func(t *testing.T, response *admissionv1beta1.AdmissionResponse) // Logic to check the response
}

// ValidationWebhookSucceeded is a helper function to verify that the validation webhook accepted the request.
func ValidationWebhookSucceeded(t *testing.T, response *admissionv1beta1.AdmissionResponse) {
	require.True(t, response.Allowed, "Request denied: %s", response.Result.Reason)
}

// ValidationWebhookFailed is a helper function to verify that the validation webhook rejected the request.
func ValidationWebhookFailed(causeRegexes ...string) func(*testing.T, *admissionv1beta1.AdmissionResponse) {
	return func(t *testing.T, response *admissionv1beta1.AdmissionResponse) {
		require.False(t, response.Allowed)
		reason := string(response.Result.Reason)
		for _, cr := range causeRegexes {
			match, err := regexp.MatchString(cr, reason)
			require.NoError(t, err, "Match '%s' returned error: %v", cr, err)
			require.True(t, match, "[%s] is not present in [%s]", cr, reason)
		}
	}
}

// RunValidationWebhookTests runs a series of ValidationWebhookTestCases
func RunValidationWebhookTests(t *testing.T, gvk metav1.GroupVersionKind, validator admission.Validator, tests ...ValidationWebhookTestCase) {
	require.NoError(t, scheme.SetupScheme())
	decoder := serializer.NewCodecFactory(clientgoscheme.Scheme).UniversalDeserializer()

	webhook := admission.ValidatingWebhookFor(validator)
	require.NoError(t, webhook.InjectScheme(clientgoscheme.Scheme))
	require.NoError(t, webhook.InjectLogger(log))

	server := httptest.NewServer(webhook)
	defer server.Close()

	client := server.Client()

	for _, tt := range tests {
		tc := tt
		t.Run(tc.Name, func(t *testing.T) {
			uid := tc.Name
			payload := &admissionv1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{Kind: "AdmissionReview"},
				Request: &admissionv1beta1.AdmissionRequest{
					UID:       types.UID(uid),
					Kind:      gvk,
					Resource:  metav1.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: gvk.Kind},
					Operation: tc.Operation,
					Object:    runtime.RawExtension{Raw: tc.Object(t, uid)},
				},
			}

			if tc.Operation == admissionv1beta1.Update {
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
					_, _ = io.Copy(ioutil.Discard, resp.Body)
					resp.Body.Close()
				}
			}()

			response := decodeResponse(t, decoder, resp.Body)
			tc.Check(t, response)

		})
	}
}

func decodeResponse(t *testing.T, decoder runtime.Decoder, body io.Reader) *admissionv1beta1.AdmissionResponse {
	t.Helper()

	responseBytes, err := ioutil.ReadAll(body)
	require.NoError(t, err, "Failed to read response body")

	response := &admissionv1beta1.AdmissionReview{}
	_, _, err = decoder.Decode(responseBytes, nil, response)
	require.NoError(t, err, "Failed to decode response")

	return response.Response
}
