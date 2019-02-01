package test

import (
	"fmt"
	"time"
)

// Millis calculates milliseconds from the given date string with layout 2006-01-02. Panics! For testing purposes only.
func Millis(dateStr string) int64 {
	layout := "2006-01-02"
	parsed, err := time.Parse(layout, dateStr)
	if err != nil {
		panic(fmt.Sprintf("incorrect test setup can't parse date %v", err))
	}
	return ToMillis(parsed)
}

func ToMillis(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}
