package today

import "testing"

func TestStatusForStart(t *testing.T) {
	t.Parallel()

	plannedAt := "2026-11-05T09:00:00Z"
	tests := []struct {
		name    string
		startAt *string
		want    ItemStatus
	}{
		{name: "missing start time", startAt: nil, want: ItemStatusUnplanned},
		{name: "empty start time", startAt: stringPointer(""), want: ItemStatusUnplanned},
		{name: "planned start time", startAt: &plannedAt, want: ItemStatusPlanned},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := StatusForStart(test.startAt); got != test.want {
				t.Fatalf("StatusForStart() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRestoredStatus(t *testing.T) {
	t.Parallel()

	plannedAt := "2026-11-05T09:00:00Z"
	if got := RestoredStatus(&plannedAt); got != ItemStatusPlanned {
		t.Fatalf("RestoredStatus() = %q, want %q", got, ItemStatusPlanned)
	}
	if got := RestoredStatus(nil); got != ItemStatusUnplanned {
		t.Fatalf("RestoredStatus() = %q, want %q", got, ItemStatusUnplanned)
	}
}

func stringPointer(value string) *string {
	return &value
}
