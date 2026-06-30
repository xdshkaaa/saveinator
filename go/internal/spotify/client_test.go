package spotify

import (
	"testing"
	"time"
)

func TestNewClientDefaultTimeout(t *testing.T) {
	t.Parallel()
	c := NewClient("id", "secret", 0)
	if c.timeout != 15*time.Second {
		t.Fatalf("timeout = %v, want 15s", c.timeout)
	}
}

func TestClientEnabled(t *testing.T) {
	t.Parallel()
	if NewClient("", "", 10).Enabled() {
		t.Fatal("expected disabled without credentials")
	}
	if !NewClient("id", "secret", 10).Enabled() {
		t.Fatal("expected enabled with credentials")
	}
}

func TestBasicAuth(t *testing.T) {
	t.Parallel()
	got := basicAuth("client", "secret")
	if got == "" || got == "client:secret" {
		t.Fatalf("basicAuth = %q", got)
	}
}
