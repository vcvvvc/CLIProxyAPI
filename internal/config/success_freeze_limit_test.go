package config

import "testing"

func TestParseConfigBytesSuccessFreezeLimit(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte("success-freeze-limit: 120\n"))
	if err != nil {
		t.Fatalf("ParseConfigBytes() error = %v", err)
	}
	if cfg.SuccessFreezeLimit != 120 {
		t.Fatalf("SuccessFreezeLimit = %d, want 120", cfg.SuccessFreezeLimit)
	}
	if _, err = ParseConfigBytes([]byte("success-freeze-limit: invalid\n")); err == nil {
		t.Fatal("ParseConfigBytes() error = nil, want invalid integer error")
	}
}
