package handler

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed admin.go
var adminGoSource string

func TestOnAdminCallbackHandlesBroadcasts(t *testing.T) {
	t.Parallel()
	if !strings.Contains(adminGoSource, `case "broadcasts":`) {
		t.Fatal(`onAdminCallback must handle case "broadcasts": admin|broadcasts is matched by the admin| prefix handler first`)
	}
}
