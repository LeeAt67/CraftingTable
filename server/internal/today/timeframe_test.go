package today

import (
	"testing"
	"time"
)

func TestResolveDayRangeUsesRequestedTimezone(t *testing.T) {
	t.Parallel()

	now := mustParseTime(t, "2026-11-05T12:00:00Z")
	dayRange, message := ResolveDayRange("2026-11-05", "Asia/Shanghai", now)
	if message != "" {
		t.Fatalf("ResolveDayRange() message = %q, want empty", message)
	}
	if dayRange.Start != "2026-11-04T16:00:00.000Z" {
		t.Fatalf("Start = %q", dayRange.Start)
	}
	if dayRange.End != "2026-11-05T16:00:00.000Z" {
		t.Fatalf("End = %q", dayRange.End)
	}
}

func TestResolveDayRangeDefaultsToNowInTimezone(t *testing.T) {
	t.Parallel()

	now := mustParseTime(t, "2026-11-05T20:00:00Z")
	dayRange, message := ResolveDayRange("", "Asia/Shanghai", now)
	if message != "" {
		t.Fatalf("ResolveDayRange() message = %q, want empty", message)
	}
	if dayRange.Start != "2026-11-05T16:00:00.000Z" {
		t.Fatalf("Start = %q", dayRange.Start)
	}
	if dayRange.End != "2026-11-06T16:00:00.000Z" {
		t.Fatalf("End = %q", dayRange.End)
	}
}

func TestResolveDayRangeFallsBackToNowLocation(t *testing.T) {
	t.Parallel()

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := mustParseTime(t, "2026-11-05T12:00:00Z").In(shanghai)
	dayRange, message := ResolveDayRange("", "", now)
	if message != "" {
		t.Fatalf("ResolveDayRange() message = %q, want empty", message)
	}
	if dayRange.Start != "2026-11-04T16:00:00.000Z" {
		t.Fatalf("Start = %q", dayRange.Start)
	}
	if dayRange.End != "2026-11-05T16:00:00.000Z" {
		t.Fatalf("End = %q", dayRange.End)
	}
}

func TestResolveDayRangeHandlesDaylightSavingTransition(t *testing.T) {
	t.Parallel()

	now := mustParseTime(t, "2026-03-08T12:00:00Z")
	dayRange, message := ResolveDayRange("2026-03-08", "America/New_York", now)
	if message != "" {
		t.Fatalf("ResolveDayRange() message = %q, want empty", message)
	}
	if dayRange.Start != "2026-03-08T05:00:00.000Z" {
		t.Fatalf("Start = %q", dayRange.Start)
	}
	if dayRange.End != "2026-03-09T04:00:00.000Z" {
		t.Fatalf("End = %q", dayRange.End)
	}
}

func TestResolveDayRangeRejectsInvalidParameters(t *testing.T) {
	t.Parallel()

	now := mustParseTime(t, "2026-11-05T12:00:00Z")
	tests := []struct {
		name     string
		date     string
		timezone string
		want     string
	}{
		{name: "invalid date", date: "2026-13-01", want: "date 必须是 YYYY-MM-DD"},
		{name: "date with time", date: "2026-11-05T00:00:00Z", want: "date 必须是 YYYY-MM-DD"},
		{name: "invalid timezone", timezone: "Mars/Olympus", want: "tz 必须是 IANA 时区名称"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, message := ResolveDayRange(test.date, test.timezone, now); message != test.want {
				t.Fatalf("message = %q, want %q", message, test.want)
			}
		})
	}
}

func TestFormatTimestampUsesFixedWidthUTC(t *testing.T) {
	t.Parallel()

	value := mustParseTime(t, "2026-11-05T01:30:00.123456789Z")
	if got := FormatTimestamp(value); got != "2026-11-05T01:30:00.123Z" {
		t.Fatalf("FormatTimestamp() = %q", got)
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   *string
		want *string
		bad  bool
	}{
		{name: "nil", in: nil},
		{name: "blank", in: stringPointer("   ")},
		{name: "utc", in: stringPointer("2026-11-05T01:30:00Z"), want: stringPointer("2026-11-05T01:30:00.000Z")},
		{name: "offset", in: stringPointer("2026-11-05T09:30:00+08:00"), want: stringPointer("2026-11-05T01:30:00.000Z")},
		{name: "nanoseconds", in: stringPointer("2026-11-05T01:30:00.123456789Z"), want: stringPointer("2026-11-05T01:30:00.123Z")},
		{name: "padded", in: stringPointer("  2026-11-05T01:30:00Z  "), want: stringPointer("2026-11-05T01:30:00.000Z")},
		{name: "invalid", in: stringPointer("tomorrow"), bad: true},
		{name: "date only", in: stringPointer("2026-11-05"), bad: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, message := NormalizeTimestamp(test.in)
			if test.bad {
				if message == "" {
					t.Fatalf("expected message for %v", pointerValue(test.in))
				}
				return
			}
			if message != "" {
				t.Fatalf("message = %q, want empty", message)
			}
			if !samePointer(got, test.want) {
				t.Fatalf("got = %v, want %v", pointerValue(got), pointerValue(test.want))
			}
		})
	}
}

func TestNormalizeOptionalTimestampKeepsClearSignal(t *testing.T) {
	t.Parallel()

	cleared, message := NormalizeOptionalTimestamp(stringPointer(""))
	if message != "" {
		t.Fatalf("message = %q, want empty", message)
	}
	if cleared == nil || *cleared != "" {
		t.Fatalf("cleared = %v, want empty sentinel", pointerValue(cleared))
	}

	if value, message := NormalizeOptionalTimestamp(nil); value != nil || message != "" {
		t.Fatalf("nil value = %v, message = %q", pointerValue(value), message)
	}

	value, message := NormalizeOptionalTimestamp(stringPointer("2026-11-05T09:30:00+08:00"))
	if message != "" {
		t.Fatalf("message = %q, want empty", message)
	}
	if value == nil || *value != "2026-11-05T01:30:00.000Z" {
		t.Fatalf("value = %v", pointerValue(value))
	}
}

func TestTimeRangeProblem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		startAt *string
		endAt   *string
		want    string
	}{
		{name: "no times"},
		{name: "only start", startAt: stringPointer("2026-11-05T09:00:00Z")},
		{
			name:    "valid range",
			startAt: stringPointer("2026-11-05T09:00:00Z"),
			endAt:   stringPointer("2026-11-05T10:00:00Z"),
		},
		{
			name:  "end without start",
			endAt: stringPointer("2026-11-05T10:00:00Z"),
			want:  "填写结束时间时必须填写开始时间",
		},
		{
			name:    "end before start",
			startAt: stringPointer("2026-11-05T09:00:00Z"),
			endAt:   stringPointer("2026-11-05T08:00:00Z"),
			want:    "结束时间必须晚于开始时间",
		},
		{
			name:    "end equals start",
			startAt: stringPointer("2026-11-05T09:00:00Z"),
			endAt:   stringPointer("2026-11-05T09:00:00Z"),
			want:    "结束时间必须晚于开始时间",
		},
		{
			name:    "offset range",
			startAt: stringPointer("2026-11-05T09:00:00+08:00"),
			endAt:   stringPointer("2026-11-05T01:30:00Z"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := TimeRangeProblem(test.startAt, test.endAt); got != test.want {
				t.Fatalf("TimeRangeProblem() = %q, want %q", got, test.want)
			}
		})
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse %s: %v", value, err)
	}
	return parsed
}

func pointerValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func samePointer(got, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}
