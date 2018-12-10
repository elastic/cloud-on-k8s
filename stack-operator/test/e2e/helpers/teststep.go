package helpers

import (
	"fmt"
	"testing"
)

// TestStep represents a single test
type TestStep struct {
	Name string
	Test func(t *testing.T)
}

// TestStepList defines a list of TestStep
type TestStepList []TestStep

// WithSteps appends the given TestSteps to the TestStepList
func (l TestStepList) WithSteps(testSteps ...TestStep) TestStepList {
	return append(l, testSteps...)
}

// TestSuite allows running a serie of test steps
type TestSuite struct {
	tsl TestStepList
}

// WithSteps creates a new test suite with the given TestSteps appended
// to the current ones
func (ts TestSuite) WithSteps(testSteps ...TestStep) *TestSuite {
	return &TestSuite{
		tsl: ts.tsl.WithSteps(testSteps...),
	}
}

// RunSequential runs the TestSuite tests sequentially,
// and fails fast on first error
func (ts TestSuite) RunSequential(t *testing.T) {
	for _, ts := range ts.tsl {
		if !t.Run(ts.Name, ts.Test) {
			fmt.Println("Test failure. Stopping early.")
			t.FailNow()
		}
	}
}
