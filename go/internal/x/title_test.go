package x

import "testing"

func TestDisplayTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/tmp/saveinator-x/McGriller 🦆 - https://t.co/VELyrgEbTN_2070912405048332288.mp4",
			want: "McGriller 🦆",
		},
		{
			path: "/tmp/saveinator-x/Cool tweet text_2070912435977146763.mp4",
			want: "Cool tweet text",
		},
		{
			path: "/tmp/saveinator-x/Author - https://t.co/abc123_1234567890123456789.mp4",
			want: "Author",
		},
		{
			path: "/tmp/saveinator-x/https://t.co/onlylink_1234567890123456789.mp4",
			want: "",
		},
		{
			path: "/tmp/saveinator-x/Real caption with_underscores_1234567890123456789.mp4",
			want: "Real caption with_underscores",
		},
	}
	for _, tc := range tests {
		if got := DisplayTitle(tc.path); got != tc.want {
			t.Fatalf("DisplayTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestCleanRawTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{in: "hello world", want: "hello world"},
		{in: "McGriller 🦆 - https://t.co/VELyrgEbTN", want: "McGriller 🦆"},
		{in: "https://t.co/abc123", want: ""},
		{in: "text – https://t.co/xyz", want: "text"},
	}
	for _, tc := range tests {
		if got := CleanRawTitle(tc.in); got != tc.want {
			t.Fatalf("CleanRawTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
