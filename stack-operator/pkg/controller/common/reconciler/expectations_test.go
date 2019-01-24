package reconciler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

var nsn1 = types.NamespacedName{
	Namespace: "namespace",
	Name:      "name",
}

var nsn2 = types.NamespacedName{
	Namespace: "namespace",
	Name:      "name2",
}

func checkExpectations(t *testing.T, e *Expectations, namespacedName types.NamespacedName, expectedCreations int64, expectedDeletions int64) {
	// check creations and deletions counters
	actualCreations, actualDeletions := e.get(namespacedName)
	require.Equal(t, expectedCreations, actualCreations)
	require.Equal(t, expectedDeletions, actualDeletions)
	// check expectations fullfilled
	expectedFulfilled := false
	if expectedCreations == 0 && expectedDeletions == 0 {
		expectedFulfilled = true
	}
	require.Equal(t, expectedFulfilled, e.Fullfilled(namespacedName))
}

func TestExpectationsTTL(t *testing.T) {
	// validate default behaviour with default TTL
	exp := NewExpectations()
	exp.ExpectCreation(nsn1)
	checkExpectations(t, exp, nsn1, 1, 0)
	// same test, but with a custom short TTL
	exp = NewExpectations()
	exp.ttl = 1 * time.Nanosecond
	exp.ExpectCreation(nsn1)
	// counters should be reset and expectations fullfilled
	// once TTL is reached
	time.Sleep(2 * time.Nanosecond)
	checkExpectations(t, exp, nsn1, 0, 0)
}

func TestExpectations(t *testing.T) {
	// tests are performing operations and checks on the same expectations object,
	// with state preserved between tests
	e := NewExpectations()
	tests := []struct {
		name     string
		events   func(e *Expectations)
		expected map[types.NamespacedName][2]int64 // namespacedName -> [creations, deletions]
	}{
		{
			name:   "empty",
			events: func(e *Expectations) {},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{0, 0},
				nsn2: [2]int64{0, 0},
			},
		},
		{
			name: "add an expected creation for nsn1",
			events: func(e *Expectations) {
				e.ExpectCreation(nsn1)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{1, 0},
				nsn2: [2]int64{0, 0},
			},
		},
		{
			name: "add 2 more expected creations for nsn1",
			events: func(e *Expectations) {
				e.ExpectCreation(nsn1)
				e.ExpectCreation(nsn1)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{3, 0},
				nsn2: [2]int64{0, 0},
			},
		},
		{
			name: "add an expected creation for nsn2",
			events: func(e *Expectations) {
				e.ExpectCreation(nsn2)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{3, 0},
				nsn2: [2]int64{1, 0},
			},
		},
		{
			name: "observe creation for nsn1",
			events: func(e *Expectations) {
				e.CreationObserved(nsn1)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{2, 0},
				nsn2: [2]int64{1, 0},
			},
		},
		{
			name: "observe 2 creations for nsn1",
			events: func(e *Expectations) {
				e.CreationObserved(nsn1)
				e.CreationObserved(nsn1)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{0, 0},
				nsn2: [2]int64{1, 0},
			},
		},
		{
			name: "observe creation for nsn2",
			events: func(e *Expectations) {
				e.CreationObserved(nsn2)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{0, 0},
				nsn2: [2]int64{0, 0},
			},
		},
		{
			name: "observe creation when counter is already at 0 should be a no-op",
			events: func(e *Expectations) {
				e.CreationObserved(nsn1)
			},
			expected: map[types.NamespacedName][2]int64{
				nsn1: [2]int64{0, 0},
				nsn2: [2]int64{0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.events(e)
			for nsn, expectationsSlice := range tt.expected {
				checkExpectations(t, e, nsn, expectationsSlice[0], expectationsSlice[1])
			}
		})
	}
}
