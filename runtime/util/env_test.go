package util

import "testing"

func TestGetEnv(t *testing.T) {
	t.Setenv("XI_TEST_KEY", "value")
	if got := GetEnv("XI_TEST_KEY", "fallback"); got != "value" {
		t.Fatalf("GetEnv set: got %q, want %q", got, "value")
	}
	if got := GetEnv("XI_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("GetEnv unset: got %q, want %q", got, "fallback")
	}
}

func TestParseInt32(t *testing.T) {
	if got := ParseInt32("42", -1); got != 42 {
		t.Fatalf("ParseInt32 valid: got %d, want 42", got)
	}
	if got := ParseInt32("nope", -1); got != -1 {
		t.Fatalf("ParseInt32 invalid: got %d, want -1", got)
	}
}
