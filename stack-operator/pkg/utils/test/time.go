package test

import (
	"fmt"
	"time"
)

// Time is a convenience function to create a 0 hour 0 minute time value from a string with layourt 2006-01-02. Panics!
func Time(dateStr string) time.Time {
	layout := "2006-01-02"
	parsed, err := time.Parse(layout, dateStr)
	if err != nil {
		panic(fmt.Sprintf("incorrect test setup can't parse date %v", err))
	}
	return parsed
}

// Millis calculates milliseconds from the given date string with layout 2006-01-02. Panics! For testing purposes only.
func Millis(dateStr string) int64 {
	return ToMillis(Time(dateStr))
}

func ToMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}
