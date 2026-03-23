// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1_test

import (
	"fmt"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/test"
)

func TestWebhook(t *testing.T) {
	testCases := []test.ValidationWebhookTestCase{
		{
			Name:      "create-valid-standalone",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-fleet-mode-missing-policyID",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Mode = agentv1alpha1.AgentFleetMode
				a.Spec.FleetServerEnabled = true
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				fmt.Sprintf("%s %s/%s: %s", agentv1alpha1.Kind, "", "webhook-test", agentv1alpha1.MissingPolicyIDMessage),
				`Version 7.17.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create-fleet-mode-with-policyID",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Mode = agentv1alpha1.AgentFleetMode
				a.Spec.FleetServerEnabled = true
				a.Spec.PolicyID = "my-policy"
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-standalone-no-policyID-no-warning",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Mode = agentv1alpha1.AgentStandaloneMode
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceeded,
		},
		{
			Name:      "create-deprecated-version",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Version = "7.14.0"
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				`Version 7.14.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "create-fleet-deprecated-version-and-missing-policyID",
			Operation: admissionv1.Create,
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Version = "7.14.0"
				a.Spec.Mode = agentv1alpha1.AgentFleetMode
				a.Spec.FleetServerEnabled = true
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookSucceededWithWarnings(
				fmt.Sprintf("%s %s/%s: %s", agentv1alpha1.Kind, "", "webhook-test", agentv1alpha1.MissingPolicyIDMessage),
				`Version 7.14.0 is EOL and support for it will be removed in a future release of the ECK operator`,
			),
		},
		{
			Name:      "deprecated-version-downgrade-warning-and-denial",
			Operation: admissionv1.Update,
			OldObject: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Version = "7.12.0"
				return test.MustMarshalJSON(t, a)
			},
			Object: func(t *testing.T, uid string) []byte {
				t.Helper()
				a := mkAgent(uid)
				a.Spec.Version = "7.10.0"
				return test.MustMarshalJSON(t, a)
			},
			Check: test.ValidationWebhookFailedWithWarnings(
				[]string{`spec.version: Forbidden: Version downgrades are not supported`},
				[]string{`Version 7.10.0 is EOL and support for it will be removed in a future release of the ECK operator`},
			),
		},
	}

	handler := test.NewValidationWebhookHandler(agentv1alpha1.Validate)
	gvk := metav1.GroupVersionKind{Group: agentv1alpha1.GroupVersion.Group, Version: agentv1alpha1.GroupVersion.Version, Kind: agentv1alpha1.Kind}
	test.RunValidationWebhookTests(t, gvk, "agents", handler, testCases...)
}

func mkAgent(uid string) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-test",
			UID:  types.UID(uid),
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:   "7.17.0",
			DaemonSet: &agentv1alpha1.DaemonSetSpec{},
		},
	}
}
