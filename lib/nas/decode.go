package nas

import (
	"strconv"
	"strings"
	"time"
)

// expiry is when a certificate runs out, from the line of text the NAS writes it as. The NAS writes it in its own time zone without naming it, so
// a time with no zone is taken to be in zone. Text that cannot be read is the zero time: a date the page cannot show is left off it.
func expiry(until string, zone *time.Location) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, until); err == nil {
		return t.UTC()
	}

	for _, layout := range []string{time.ANSIC, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, until, zone); err == nil {
			return t.UTC()
		}
	}

	return time.Time{}
}

// number is a number the NAS writes as one or as text: a pool's fragmentation is "14" and a dataset's compression "1.04". It is false for nothing
// at all and for text that is no number, "-" say.
func number(v any) (float64, bool) {
	switch v := v.(type) {
	case float64:
		return v, true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), "x"), 64)
		return n, err == nil
	}

	return 0, false
}

// size is a dataset's property as a size, from the value the NAS has already made a number of, and 0 when it is not given.
func size(parsed any) int64 {
	n, _ := number(parsed)
	return int64(n)
}

// limit is a dataset's property as a size that may not be set: nil for none, which the NAS writes as nothing or as 0.
func limit(parsed any) *int64 {
	n, ok := number(parsed)
	if !ok || n <= 0 {
		return nil
	}

	return new(int64(n))
}

// ratio is a percentage as a share of the whole, from 0 to 1 and never outside it: a scrub can say it is 100.4% done.
func ratio(percent float64) float64 {
	return min(max(percent/100, 0), 1)
}
