// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"time"
)

// MustParseTime is a convenience function to create a 0 hour 0 minute time value from a string with layout 2006-01-02.
// Panics! Convenience function for testing purposes only.
func MustParseTime(dateStr string) time.Time {
	layout := "2006-01-02"
	parsed, err := time.Parse(layout, dateStr)
	if err != nil {
		panic(fmt.Sprintf("incorrect test setup can't parse date %v", err))
	}
	return parsed
}

// Millis calculates milliseconds from the given date string with layout 2006-01-02.
// Panics! Convenience function for testing purposes only.
func Millis(dateStr string) int64 {
	return ToMillis(MustParseTime(dateStr))
}

// ToMillis returns t in milliseconds.
func ToMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}
