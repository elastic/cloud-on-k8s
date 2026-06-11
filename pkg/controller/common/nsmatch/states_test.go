// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNamespaceStates(t *testing.T) {
	t.Run("Swap unknown namespace returns not known", func(t *testing.T) {
		s := NewNamespaceStates()
		wasMatching, known := s.Swap("ns-a", true)
		assert.False(t, known, "first call must report unknown")
		assert.False(t, wasMatching, "zero value returned for unknown namespace")
	})

	t.Run("Swap known namespace returns previous value", func(t *testing.T) {
		s := NewNamespaceStates()
		s.Swap("ns-a", true) // seed

		wasMatching, known := s.Swap("ns-a", false)
		assert.True(t, known)
		assert.True(t, wasMatching, "previous value was true")
	})

	t.Run("Swap same value round-trips correctly", func(t *testing.T) {
		s := NewNamespaceStates()
		s.Swap("ns-a", true)

		wasMatching, known := s.Swap("ns-a", true)
		assert.True(t, known)
		assert.True(t, wasMatching)
	})

	t.Run("Forget makes namespace unknown again", func(t *testing.T) {
		s := NewNamespaceStates()
		s.Swap("ns-a", true)
		s.Forget("ns-a")

		_, known := s.Swap("ns-a", false)
		assert.False(t, known, "after Forget the namespace must be unknown again")
	})

	t.Run("Forget unknown namespace does not panic", func(t *testing.T) {
		s := NewNamespaceStates()
		s.Forget("never-seen")
	})

	t.Run("independent namespaces do not share state", func(t *testing.T) {
		s := NewNamespaceStates()
		s.Swap("ns-a", true)
		s.Swap("ns-b", false)

		wasA, knownA := s.Swap("ns-a", false)
		wasB, knownB := s.Swap("ns-b", true)

		assert.True(t, knownA)
		assert.True(t, wasA)
		assert.True(t, knownB)
		assert.False(t, wasB)
	})

	t.Run("concurrent Swap and Forget do not race", func(t *testing.T) {
		s := NewNamespaceStates()
		const goroutines = 10

		var wg sync.WaitGroup
		wg.Add(goroutines * 2)
		for range goroutines {
			go func() {
				defer wg.Done()
				s.Swap("ns-concurrent", true)
			}()
			go func() {
				defer wg.Done()
				s.Forget("ns-concurrent")
			}()
		}
		wg.Wait()
	})
}
