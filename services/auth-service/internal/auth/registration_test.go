package auth

import "testing"

func TestRegistrationDoesNotRequireOrganization(t *testing.T) {
	if !validRegistrationInput("user@example.com", "user", "User", "correct horse battery staple") {
		t.Fatal("registration without organization should be valid")
	}
}
