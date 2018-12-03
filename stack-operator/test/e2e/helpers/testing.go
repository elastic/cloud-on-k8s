package helpers

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
)

const (
	defaultRetryDelay = 3 * time.Second
	defaultTimeout    = 3 * time.Minute
)

// ExitOnErr exits with code 1 if the given error is not nil
func ExitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println("Exiting.")
		os.Exit(1)
	}
}

// Eventually runs the given function until success,
// with a default timeout
func Eventually(f func() error) func(*testing.T) {
	return func(t *testing.T) {
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, defaultTimeout, defaultRetryDelay)
		assert.NoError(t, err)
	}
}

// TestCase represents a single test
type TestCase struct {
	Name string
	Test func(t *testing.T)
}

// TestSuite allows running a serie of test cases
type TestSuite struct {
	tc []TestCase
}

// WithTestCases creates a new test suite with the given TestCases appended
// to the current ones
func (ts TestSuite) WithTestCases(testCases ...TestCase) *TestSuite {
	return &TestSuite{
		tc: append(ts.tc, testCases...),
	}
}

// RunSequential runs the TestSuite tests sequentially,
// and fails fast on first error
func (ts TestSuite) RunSequential(t *testing.T) {
	for _, tc := range ts.tc {
		if !t.Run(tc.Name, tc.Test) {
			fmt.Println("Test failure. Stopping early.")
			t.FailNow()
		}
	}
}
