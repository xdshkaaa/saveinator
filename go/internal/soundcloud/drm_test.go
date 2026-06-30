package soundcloud

import (
	"errors"
	"testing"
)

func TestIsDRMError(t *testing.T) {
	t.Parallel()
	err := errors.New("exit status 1")
	out := []byte("ERROR: [soundcloud] 2328944003: This video is DRM protected")
	if !isDRMError(err, out) {
		t.Fatal("expected drm error")
	}
	if isDRMError(errors.New("404 not found"), []byte("HTTP Error 404")) {
		t.Fatal("expected non-drm error")
	}
}
