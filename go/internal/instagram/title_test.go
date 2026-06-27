package instagram

import "testing"

func TestDisplayTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/tmp/Video by lanadsa2803_DXMZOuiDK07.mp4",
			want: "",
		},
		{
			path: "/tmp/Amazing reel caption_DXMZOuiDK07.mp4",
			want: "Amazing reel caption",
		},
		{
			path: "/tmp/Photo by someuser_ABCdef12345.mp4",
			want: "",
		},
		{
			path: "/tmp/Caption with_underscores_DXMZOuiDK07.mp4",
			want: "Caption with_underscores",
		},
		{
			path: "/tmp/Real title_DaAl-AKqLRF.mp4",
			want: "Real title",
		},
	}
	for _, tc := range tests {
		if got := DisplayTitle(tc.path); got != tc.want {
			t.Fatalf("DisplayTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
