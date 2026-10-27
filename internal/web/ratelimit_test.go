package web

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresSpoofedLeadingXFF(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP = %q, want the proxy-appended last entry", got)
	}
}
