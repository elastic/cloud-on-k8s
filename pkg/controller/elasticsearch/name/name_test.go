// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidate(t *testing.T) {
	testCases := []struct {
		name          string
		esName        string
		nodeSpecNames []string
		wantErr       bool
		wantErrMsg    string
	}{
		{
			name:          "valid configuration",
			esName:        "test-es",
			nodeSpecNames: []string{"default", "ha"},
			wantErr:       false,
		},
		{
			name:          "long ES name",
			esName:        "extremely-long-winded-and-unnecessary-name-for-elasticsearch",
			nodeSpecNames: []string{"default", "ha"},
			wantErr:       true,
			wantErrMsg:    "name exceeds maximum allowed length",
		},
		{
			name:          "long nodeSpec name",
			esName:        "test-es",
			nodeSpecNames: []string{"default", "extremely-long-nodespec-name-for-no-particular-reason"},
			wantErr:       true,
			wantErrMsg:    "suffix exceeds max length",
		},
		{
			name:          "invalid characters in nodeSpec name",
			esName:        "test-es",
			nodeSpecNames: []string{"default", "my_ha_set"},
			wantErr:       true,
			wantErrMsg:    "invalid nodeSpec name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := v1alpha1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.esName,
					Namespace: "test",
				},
				Spec: v1alpha1.ElasticsearchSpec{},
			}

			for _, nodeSpecName := range tc.nodeSpecNames {
				es.Spec.Nodes = append(es.Spec.Nodes, v1alpha1.NodeSpec{Name: nodeSpecName, NodeCount: 10})
			}

			err := Validate(es)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
