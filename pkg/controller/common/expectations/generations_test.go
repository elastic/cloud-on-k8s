// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"

	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func newStatefulSet(name string, uid types.UID, generation int64) appsv1.StatefulSet {
	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "ns",
			Name:       name,
			UID:        uid,
			Generation: generation,
		},
	}
}

func TestExpectedStatefulSetUpdates_GenerationsSatisfied(t *testing.T) {
	sset1 := newStatefulSet("sset1", uuid.NewUUID(), 3)
	sset1DifferentUID := newStatefulSet("sset1", uuid.NewUUID(), 3)
	sset1HigherGen := newStatefulSet("sset1", sset1.UID, 7)
	sset2 := newStatefulSet("sset2", uuid.NewUUID(), 12)
	sset2HigherGen := newStatefulSet("sset2", sset2.UID, 13)

	tests := []struct {
		name                    string
		resources               []runtime.Object
		expectGenerations       []appsv1.StatefulSet
		wantSatisfied           bool
		wantExpectedGenerations map[types.NamespacedName]ResourceGeneration
	}{
		{
			name:                    "no generation expected",
			resources:               []runtime.Object{&sset1, &sset2},
			expectGenerations:       nil,
			wantSatisfied:           true,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{},
		},
		{
			name:              "one generation expected, unsatisfied",
			resources:         []runtime.Object{&sset1, &sset2},
			expectGenerations: []appsv1.StatefulSet{sset2HigherGen},
			wantSatisfied:     false,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{
				k8s.ExtractNamespacedName(&sset2HigherGen): {
					UID:        sset2HigherGen.UID,
					Generation: sset2HigherGen.Generation,
				}},
		},
		{
			name:                    "one generation expected, satisfied",
			resources:               []runtime.Object{&sset1, &sset2HigherGen},
			expectGenerations:       []appsv1.StatefulSet{sset2HigherGen},
			wantSatisfied:           true,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{},
		},
		{
			name:                    "two generations expected, satisfied",
			resources:               []runtime.Object{&sset1HigherGen, &sset2HigherGen},
			expectGenerations:       []appsv1.StatefulSet{sset1HigherGen, sset2HigherGen},
			wantSatisfied:           true,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{},
		},
		{
			name:              "two generations expected, one unsatisfied",
			resources:         []runtime.Object{&sset1HigherGen, &sset2},
			expectGenerations: []appsv1.StatefulSet{sset1HigherGen, sset2HigherGen},
			wantSatisfied:     false,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{
				k8s.ExtractNamespacedName(&sset2HigherGen): {
					UID:        sset2HigherGen.UID,
					Generation: sset2HigherGen.Generation,
				}},
		},
		{
			name:                    "expecting a generation for a StatefulSet that does not exist anymore: satisfied",
			resources:               []runtime.Object{},
			expectGenerations:       []appsv1.StatefulSet{sset1},
			wantSatisfied:           true,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{},
		},
		{
			name:                    "expecting a generation for a StatefulSet that was replaced: satisfied",
			resources:               []runtime.Object{&sset1},
			expectGenerations:       []appsv1.StatefulSet{sset1DifferentUID},
			wantSatisfied:           true,
			wantExpectedGenerations: map[types.NamespacedName]ResourceGeneration{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controllerscheme.SetupScheme()
			client := k8s.NewFakeClient(tt.resources...)
			e := NewExpectedStatefulSetUpdates(client)
			for i := range tt.expectGenerations {
				e.ExpectGeneration(tt.expectGenerations[i])
				require.Contains(t, e.generations, k8s.ExtractNamespacedName(&tt.expectGenerations[i]))
			}
			pendingGenerations, err := e.PendingGenerations()
			require.NoError(t, err)
			require.Equal(t, tt.wantSatisfied, len(pendingGenerations) == 0)
			require.Equal(t, tt.wantExpectedGenerations, e.generations)
		})
	}
}
