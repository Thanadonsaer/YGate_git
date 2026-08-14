package auth

import "testing"

func TestRegistrationDoesNotRequireOrganization(t *testing.T) {
	if !validRegistrationInput("user@example.com", "user", "User", "correct horse battery staple") {
		t.Fatal("registration without organization should be valid")
	}
}

func TestNormalizeVerificationIdentifier(t *testing.T) {
	if got := normalizeVerificationIdentifier("  User@Example.COM "); got != "user@example.com" {
		t.Fatalf("normalizeVerificationIdentifier() = %q", got)
	}
}
