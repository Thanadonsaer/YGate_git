package auth

import "testing"

func TestPasswordHashingAndPolicy(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") || VerifyPassword(hash, "wrong") {
		t.Fatal("password verification mismatch")
	}
	if !ValidateNewPassword("long-enough-password") || ValidateNewPassword("too-short") {
		t.Fatal("password policy mismatch")
	}
}
