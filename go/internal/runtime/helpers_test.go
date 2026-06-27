package runtime

import "testing"

func TestSplitList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "single", raw: "1080", want: []string{"1080"}},
		{name: "comma separated", raw: "1080,720,480", want: []string{"1080", "720", "480"}},
		{name: "with spaces", raw: " 1080 , 720 , 480 ", want: []string{"1080", "720", "480"}},
		{name: "empty parts", raw: "1080,,720", want: []string{"1080", "720"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitList(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("splitList(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("splitList(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}
