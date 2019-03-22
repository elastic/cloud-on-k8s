package comparison

import (
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("mutation")

type Comparison struct {
	Match           bool
	MismatchReasons []string
}

func NewComparison(match bool, mismatchReasons ...string) Comparison {
	return Comparison{Match: match, MismatchReasons: mismatchReasons}
}

var ComparisonMatch = NewComparison(true)

func ComparisonMismatch(mismatchReasons ...string) Comparison {
	return NewComparison(false, mismatchReasons...)
}

func NewStringComparison(expected string, actual string, name string) Comparison {
	return NewComparison(expected == actual, fmt.Sprintf("%s mismatch: expected %s, actual %s", name, expected, actual))
}
