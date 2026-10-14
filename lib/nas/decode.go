package nas

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// moment is a time as the NAS writes one: {"$date": milliseconds since 1970}, the same number bare, a date in text, or nothing at all. Nothing at
// all and anything that cannot be read are the zero time: a date the page cannot show is left off it, which is no reason to lose the rest of an
// answer.
type moment struct {
	time.Time

	// unzoned is a date in text that names no time zone, which the NAS writes in its own: in puts it there
	unzoned bool
}

func (m *moment) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	m.Time = time.Time{}

	var wrapped struct {
		Date *json.Number `json:"$date"`
	}
	if len(b) > 0 && b[0] == '{' {
		if err := json.Unmarshal(b, &wrapped); err != nil || wrapped.Date == nil {
			return nil //nolint:nilerr // an object that is no date is a date not given
		}

		b = []byte(wrapped.Date.String())
	}

	if ms, err := strconv.ParseFloat(string(b), 64); err == nil {
		m.Time = time.UnixMilli(int64(ms)).UTC()
		return nil
	}

	var text string
	if err := json.Unmarshal(b, &text); err != nil {
		return nil //nolint:nilerr // neither a number nor text is a date not given
	}

	for _, layout := range []struct {
		layout string
		zoned  bool
	}{{time.RFC3339Nano, true}, {"2006-01-02T15:04:05", false}, {"2006-01-02 15:04:05", false}, {time.ANSIC, false}} {
		if t, err := time.Parse(layout.layout, text); err == nil {
			m.Time, m.unzoned = t.UTC(), !layout.zoned
			break
		}
	}

	return nil
}

// in is the time, with a date that named no time zone taken to be in this one: the NAS's own, where it writes such dates.
func (m moment) in(zone *time.Location) time.Time {
	if !m.unzoned {
		return m.Time
	}

	t := m.Time
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), zone).UTC()
}

// number is a number the NAS writes as one or as text: a pool's fragmentation is "14" and a dataset's compression "1.04". Known is false for
// nothing at all and for text that is no number, "-" say.
type number struct {
	Value float64
	Known bool
}

func (n *number) UnmarshalJSON(b []byte) error {
	*n = number{}

	if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.Trim(string(bytes.TrimSpace(b)), `"`), "x"), 64); err == nil {
		*n = number{Value: v, Known: true}
	}

	return nil
}

// property is one property of a dataset, which the NAS gives several ways round: Parsed is the one already made into a number.
type property struct {
	Parsed number `json:"parsed"`
}

// bytes is the property as a size, 0 when it is not given.
func (p property) bytes() int64 {
	return int64(p.Parsed.Value)
}

// limit is the property as a size that may not be set: nil for none, which the NAS writes as nothing or as 0.
func (p property) limit() *int64 {
	if !p.Parsed.Known || p.Parsed.Value <= 0 {
		return nil
	}

	return new(int64(p.Parsed.Value))
}

// ratio is a percentage as a share of the whole, from 0 to 1 and never outside it: a scrub can say it is 100.4% done.
func ratio(percent float64) float64 {
	return min(max(percent/100, 0), 1)
}
