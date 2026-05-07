// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-deprecated-version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "deprecated-at-lowest-supported-7-0-0",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.0.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.0.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create-8-0-0-no-deprecation-warning",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.0.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-non-deprecated-version-no-warning",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.2.3"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "invalid-version-single-cause",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.x"
				return test.MustMarshalJSON(t, b)
			},
			Check: func(t *testing.T, response *admissionv1.AdmissionResponse) {
				t.Helper()
				test.ValidationWebhookFailed(`spec.version: Invalid value: "7.x": Invalid version`)(t, response)
				require.Len(t, response.Result.Details.Causes, 1, "invalid version should produce exactly one cause, not duplicate parse errors")
			},
		},
		{
			Name:      "update-valid",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.2.3"
				return test.MustMarshalJSON(t, b)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.3.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "update-deprecated-same-version-label-change-still-warns",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, b)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				b.Labels = map[string]string{"warmed": "restart"}
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "update-from-deprecated-to-supported-clears-deprecation-warning",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, b)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.3.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "version-downgrade",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.3.0"
				return test.MustMarshalJSON(t, b)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.2.3"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookFailed(
				`spec.version: Forbidden: Version downgrades are not supported`,
			),
		},
		{
			Name:      "version-downgrade-deprecated-target",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.0.0"
				return test.MustMarshalJSON(t, b)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, b)
			},
			Check: test.ValidationWebhookFailedWithWarnings(
				[]string{`spec.version: Forbidden: Version downgrades are not supported`},
				[]string{`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`},
			),
		},
	}

	handler := test.NewValidationWebhookHandler(beatv1beta1.Validate)
	gvk := metav1.GroupVersionKind{Group: beatv1beta1.GroupVersion.Group, Version: beatv1beta1.GroupVersion.Version, Kind: beatv1beta1.Kind}
	test.RunValidationWebhookTests(t, gvk, "beats", handler, testCases...)
}

func mkBeat(uid string) *beatv1beta1.Beat {
	return &beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: beatv1beta1.BeatSpec{
			Type:      "filebeat",
			Version:   "8.2.3",
			DaemonSet: &beatv1beta1.DaemonSetSpec{},
		},
	}
}
