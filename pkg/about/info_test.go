// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package about

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const fakeOperatorNs = "elastic-system-test"
const fakeDistributionChannel = "channel-1"

func TestGetOperatorInfo(t *testing.T) {
	tests := []struct {
		name     string
		initObjs []runtime.Object
		assert   func(uuid types.UID)
	}{
		{
			name: "should create an operator uuid config map when it does not exist",
			assert: func(uuid types.UID) {
				assert.NotEqual(t, types.UID(""), uuid)
			},
		},
		{
			name: "should update an operator uuid config map when it is empty",
			initObjs: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      UUIDCfgMapName,
						Namespace: fakeOperatorNs,
					},
				},
			},
			assert: func(uuid types.UID) {
				assert.NotEqual(t, types.UID(""), uuid)
			},
		},
		{
			name: "should retrieve an operator uuid config map when it is already defined",
			initObjs: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      UUIDCfgMapName,
						Namespace: fakeOperatorNs,
					},
					Data: map[string]string{
						UUIDCfgMapKey: "01010101-0101-4242-0101-010101010101",
					},
				},
			},
			assert: func(uuid types.UID) {
				assert.Equal(t, types.UID("01010101-0101-4242-0101-010101010101"), uuid)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClientset := k8sfake.NewSimpleClientset(test.initObjs...)

			// retrieve operator info a first time
			operatorInfo, err := GetOperatorInfo(fakeClientset, fakeOperatorNs, fakeDistributionChannel)
			require.NoError(t, err)

			// the operator uuid should be defined
			uuid := operatorInfo.OperatorUUID
			test.assert(uuid)

			// retrieve operator info a second time
			operatorInfo, err = GetOperatorInfo(fakeClientset, fakeOperatorNs, fakeDistributionChannel)
			require.NoError(t, err)

			// the operator uuid should be the same than the first time
			assert.Equal(t, uuid, operatorInfo.OperatorUUID)
		})
	}
}

func TestGetOperatorInfo_DistributionChannel(t *testing.T) {
	tests := []struct {
		name                     string
		distributionChannelParam string
		wantDistributionChannel  string
	}{
		{
			name:                     "known distribution channel",
			distributionChannelParam: "helm",
			wantDistributionChannel:  "helm",
		},
		{
			name:                     "no distribution channel",
			distributionChannelParam: "",
			wantDistributionChannel:  "",
		},
		{
			name:                     "unknown distribution channel",
			distributionChannelParam: fakeDistributionChannel,
			wantDistributionChannel:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClientset := k8sfake.NewSimpleClientset()

			operatorInfo, err := GetOperatorInfo(fakeClientset, fakeOperatorNs, test.distributionChannelParam)
			require.NoError(t, err)
			require.Equal(t, test.wantDistributionChannel, operatorInfo.DistributionChannel)
		})
	}
}
