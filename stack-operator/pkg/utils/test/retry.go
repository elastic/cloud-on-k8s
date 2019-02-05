package test

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
)

// Default values to be used for testing purpose
const (
	Timeout       = time.Second * 5
	RetryInterval = time.Millisecond * 100
)

// RetryUntilSuccess calls retry.UntilSuccess with
// default timeout and retry interval,
// and asserts that no error is returned
func RetryUntilSuccess(t *testing.T, f func() error) {
	assert.NoError(t, retry.UntilSuccess(f, Timeout, RetryInterval))
}
