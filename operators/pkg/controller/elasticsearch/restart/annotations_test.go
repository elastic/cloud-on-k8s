// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package restart

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

func Test_getPhase(t *testing.T) {
	_, isSet := getPhase(corev1.Pod{})
	require.False(t, isSet)

	phase, isSet := getPhase(corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				PhaseAnnotation: string(PhaseStop),
			},
		},
	})
	require.True(t, isSet)
	require.Equal(t, PhaseStop, phase)
}

func Test_hasPhase(t *testing.T) {
	require.True(t, hasPhase(
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			PhaseAnnotation: string(PhaseStop),
		}}},
		PhaseStop))

	require.False(t, hasPhase(
		corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			PhaseAnnotation: string(PhaseStart),
		}}},
		PhaseStop))

	require.False(t,
		hasPhase(corev1.Pod{}, PhaseStop),
	)
}

func Test_filterPodsInPhase(t *testing.T) {
	pods := []corev1.Pod{
		{},
		{},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStop)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStop)}}},
	}
	expected := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
		{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}},
	}
	require.Equal(t, expected, filterPodsInPhase(pods, PhaseStart))
}

func Test_isAnnotatedForRestart(t *testing.T) {
	require.False(t, isAnnotatedForRestart(corev1.Pod{}))
	require.False(t, isAnnotatedForRestart(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{StrategyAnnotation: string(StrategyCoordinated)}}}))
	// we only care about the phase
	require.True(t, isAnnotatedForRestart(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{PhaseAnnotation: string(PhaseStart)}}}))
}

func Test_setPhase(t *testing.T) {
	// create a pod
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
		},
	}
	k := k8s.WrapClient(fake.NewFakeClient(&pod))
	err := setPhase(k, pod, PhaseStart)
	require.NoError(t, err)

	// retrieve the pod
	var updatedPod corev1.Pod
	err = k.Get(k8s.ExtractNamespacedName(&pod), &updatedPod)
	require.NoError(t, err)

	// it should have the correct annotations
	phase, set := getPhase(updatedPod)
	require.True(t, set)
	require.Equal(t, PhaseStart, phase)
}

func Test_getStrategy(t *testing.T) {
	require.Equal(t, StrategySimple, getStrategy(corev1.Pod{}))
	require.Equal(t, StrategyCoordinated, getStrategy(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{StrategyAnnotation: string(StrategyCoordinated)}}}))
}

func Test_setScheduleRestartAnnotations(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
		},
	}
	// create the pod
	k := k8s.WrapClient(fake.NewFakeClient(&pod))

	// schedule a restart
	err := setScheduleRestartAnnotations(k, pod, StrategyCoordinated, now)
	require.NoError(t, err)

	// retrieve the pod
	var updatedPod corev1.Pod
	err = k.Get(k8s.ExtractNamespacedName(&pod), &updatedPod)
	require.NoError(t, err)

	// it should have the correct annotations
	phase, set := getPhase(updatedPod)
	require.True(t, set)
	require.Equal(t, PhaseSchedule, phase)
	require.Equal(t, StrategyCoordinated, getStrategy(updatedPod))
	startTime, set := getStartTime(updatedPod)
	require.True(t, set)
	require.Equal(t, now, startTime)
}

func Test_getClusterRestartAnnotation(t *testing.T) {
	require.Equal(t, Strategy(""), getClusterRestartAnnotation(v1alpha1.Elasticsearch{}))
	require.Equal(t, StrategyCoordinated, getClusterRestartAnnotation(v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ClusterRestartAnnotation: string(StrategyCoordinated)}}}))
}

func TestAnnotateClusterForCoordinatedRestart(t *testing.T) {
	cluster := v1alpha1.Elasticsearch{}
	AnnotateClusterForCoordinatedRestart(&cluster)
	require.Equal(t, StrategyCoordinated, getClusterRestartAnnotation(cluster))
}

func Test_getStartTime(t *testing.T) {
	_, isSet := getStartTime(corev1.Pod{})
	require.False(t, isSet)

	startTime, isSet := getStartTime(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{StartTimeAnnotation: "2019-04-19T13:29:15+02:00"}}})
	require.True(t, isSet)
	expected, err := time.Parse(time.RFC3339, "2019-04-19T13:29:15+02:00")
	require.NoError(t, err)
	require.Equal(t, expected, startTime)
}
