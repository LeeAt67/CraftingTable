package docs

import (
	"errors"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	t.Parallel()

	if _, err := NormalizeQuery("   "); err != nil {
		t.Fatalf("blank query error = %v, want nil", err)
	}
	value, err := NormalizeQuery("  接口  ")
	if err != nil || value != "接口" {
		t.Fatalf("NormalizeQuery() = %q, %v", value, err)
	}
	long := make([]rune, MaxQueryRunes+1)
	for index := range long {
		long[index] = 'a'
	}
	if _, err := NormalizeQuery(string(long)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("long query error = %v, want ErrInvalidInput", err)
	}
}

func TestNormalizeLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input int
		want  int
	}{
		{input: 0, want: DefaultSearchLimit},
		{input: -5, want: DefaultSearchLimit},
		{input: 1, want: 1},
		{input: MaxSearchLimit, want: MaxSearchLimit},
		{input: 999, want: MaxSearchLimit},
	}

	for _, test := range tests {
		if got := NormalizeLimit(test.input); got != test.want {
			t.Fatalf("NormalizeLimit(%d) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestLikePatternEscapesWildcards(t *testing.T) {
	t.Parallel()

	if got := LikePattern(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("LikePattern() = %q", got)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()

	cursor := SearchCursor{
		Rank:      1,
		UpdatedAt: "2026-11-06T01:02:03.000000004Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	}
	raw := EncodeCursor(cursor)
	decoded, err := DecodeCursor(raw, "接口")
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if *decoded != cursor {
		t.Fatalf("decoded = %#v, want %#v", *decoded, cursor)
	}
}

func TestDecodeCursorRejectsMismatchedQuery(t *testing.T) {
	t.Parallel()

	raw := EncodeCursor(SearchCursor{
		Rank:      0,
		UpdatedAt: "2026-11-06T01:02:03Z",
		ID:        "6f1b0d5e-3f4a-4f7b-8c9d-0a1b2c3d4e5f",
		QueryHash: QueryHash("接口"),
	})
	if _, err := DecodeCursor(raw, "文档"); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("mismatched query error = %v, want ErrInvalidCursor", err)
	}
}

func TestDecodeCursorRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "not-base64!", "e30"} {
		if _, err := DecodeCursor(raw, "接口"); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("DecodeCursor(%q) error = %v, want ErrInvalidCursor", raw, err)
		}
	}
}
