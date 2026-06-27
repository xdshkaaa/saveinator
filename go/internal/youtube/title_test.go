package youtube

import "testing"

func TestDisplayTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/tmp/Айтишникам нельзя любить деньги_LyEXlCNMJU0_9_16.mp4",
			want: "Айтишникам нельзя любить деньги",
		},
		{
			path: "/tmp/Айтишникам нельзя любить деньги__LyEXlCNMJU0_9_16.mp4",
			want: "Айтишникам нельзя любить деньги",
		},
		{
			path: "/tmp/My Video_dQw4w9WgXcQ_16_9.mp4",
			want: "My Video",
		},
		{
			path: "/tmp/My Video_dQw4w9WgXcQ.mp4",
			want: "My Video",
		},
	}
	for _, tc := range tests {
		if got := DisplayTitle(tc.path); got != tc.want {
			t.Fatalf("DisplayTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
