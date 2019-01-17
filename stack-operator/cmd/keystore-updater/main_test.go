package main

import (
	"errors"
	"github.com/stretchr/testify/assert"

	"testing"
	"time"
)

func failing(_ chan<- struct{}) func() error {

	return func() error {
		return errors.New("boom")
	}
}

func succeeding(done chan<- struct{}) func() error {
	return func() error {
		defer func() {
			done <- struct{}{}
		}()
		return nil
	}
}

func Test_coalescingRetry(t *testing.T) {
	type args []func(chan<- struct{}) func() error
	type expected struct {
		minSuccesses      int
		upperBoundRetries int
	}
	tests := []struct {
		name     string
		expected expected
		args     args
	}{
		{
			name: "happy path",
			expected: expected{
				minSuccesses:      1,
				upperBoundRetries: 0,
			},
			args: []func(chan<- struct{}) func() error{succeeding},
		},
		{
			name: "error coalesced with success",
			expected: expected{
				minSuccesses:      1,
				upperBoundRetries: 1,
			},
			args: []func(chan<- struct{}) func() error{
				failing,
				succeeding,
			},
		},
		{
			name: "multiple errors coalesced with success",
			expected: expected{
				minSuccesses:      1,
				upperBoundRetries: 3,
			},
			args: []func(chan<- struct{}) func() error{
				failing,
				failing,
				failing,
				succeeding,
			},
		},
		{
			name: "sequence of errors and successes",
			expected: expected{
				minSuccesses:      1,
				upperBoundRetries: 2,
			},
			args: []func(chan<- struct{}) func() error{
				failing,
				succeeding,
				failing,
				succeeding,
			},
		},
		{
			name: "multiple successes",
			expected: expected{
				minSuccesses:      2,
				upperBoundRetries: 0,
			},
			args: []func(chan<- struct{}) func() error{
				succeeding,
				succeeding,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			successes := make(chan struct{}, 100)
			in := make(chan func() error)
			retries := 0
			go coalescingRetry(in, func(_ int) time.Duration {
				retries++
				return 1 * time.Nanosecond
			})
			for _, fn := range tt.args {
				in <- fn(successes)
			}
			for i := 0; i < tt.expected.minSuccesses; i++ {
				<-successes
			}
			assert.True(
				t,
				retries <= tt.expected.upperBoundRetries,
				"should make less than %d retries but was %d",
				tt.expected.upperBoundRetries,
				retries,
			)

		})
	}
}
