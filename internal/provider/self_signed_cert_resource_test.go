package provider

import (
	"testing"
	"time"
)

func TestReadyForRenewal(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expires   string
		early     int64
		wantReady bool
	}{
		{name: "before window", expires: "2026-08-11", early: 24, wantReady: false},
		{name: "inside window", expires: "2026-07-12", early: 24, wantReady: true},
		{name: "expired", expires: "2026-07-10", early: 0, wantReady: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readyForRenewal(now, tt.expires, tt.early)
			if err != nil {
				t.Fatalf("readyForRenewal returned error: %v", err)
			}
			if got != tt.wantReady {
				t.Fatalf("expected %v, got %v", tt.wantReady, got)
			}
		})
	}
}

func TestReadyForRenewal_InvalidDate(t *testing.T) {
	_, err := readyForRenewal(time.Now(), "bad-date", 0)
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}
