package docs

import "testing"

func TestNormalizeTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		kind  NodeKind
		value string
		want  string
	}{
		{name: "trims document title", kind: NodeKindDocument, value: "  接口设计  ", want: "接口设计"},
		{name: "document default", kind: NodeKindDocument, value: "   ", want: DefaultDocumentTitle},
		{name: "folder default", kind: NodeKindFolder, value: "", want: DefaultFolderTitle},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTitle(test.kind, test.value); got != test.want {
				t.Fatalf("NormalizeTitle() = %q, want %q", got, test.want)
			}
		})
	}
}
