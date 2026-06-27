package pinterest

import "testing"

func TestDisplayTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{
			path: "/tmp/pin_811985007859293841.jpg",
			want: "",
		},
		{
			path: "pin_123456789012345678.mp4",
			want: "",
		},
		{
			path: "/tmp/board_811985007859293841_image.jpg",
			want: "",
		},
		{
			path: "/tmp/board_811985007859293841_video.mp4",
			want: "",
		},
		{
			path: "/tmp/Beautiful sunset photo.jpg",
			want: "Beautiful sunset photo",
		},
	}
	for _, tc := range tests {
		if got := DisplayTitle(tc.path); got != tc.want {
			t.Fatalf("DisplayTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
