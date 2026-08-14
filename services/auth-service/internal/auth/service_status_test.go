package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestClassifyLoginStatusAfterPasswordVerification(t *testing.T) {
	tests := []struct {
		name, status string
		verified     bool
		want         error
	}{
		{name: "unverified pending account", status: "PENDING_ACCESS", want: ErrEmailUnverified},
		{name: "verified pending account", status: "PENDING_ACCESS", verified: true, want: ErrAccessPending},
		{name: "verified disabled account", status: "DISABLED", verified: true, want: ErrAccountDisabled},
		{name: "active account", status: "ACTIVE", verified: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var verifiedAt pgtype.Timestamptz
			if tt.verified {
				verifiedAt = pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}
			}
			if got := classifyLoginStatus(tt.status, verifiedAt); !errors.Is(got, tt.want) {
				t.Fatalf("classifyLoginStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}
