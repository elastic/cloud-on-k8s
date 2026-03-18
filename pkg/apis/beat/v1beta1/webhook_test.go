// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1_test

import (
	"encoding/json"
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
			Name:      "create valid",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				return serialize(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create deprecated version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "7.10.0"
				return serialize(t, b)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create non deprecated version no warning",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				b := mkBeat(uid)
				b.Spec.Version = "8.2.3"
				return serialize(t, b)
			},
			Check: test.ValidationWebhookSucceeded,
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

func serialize(t *testing.T, b *beatv1beta1.Beat) []byte {
	t.Helper()

	objBytes, err := json.Marshal(b)
	require.NoError(t, err)

	return objBytes
}
