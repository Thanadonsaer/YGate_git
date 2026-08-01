package auth

import "testing"

func TestPasswordAndTokens(t *testing.T) {
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

	token, storedHash, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	if !hashesEqual(storedHash, token) || hashesEqual(storedHash, token+"x") || string(storedHash) == token {
		t.Fatal("token hashing or constant-time comparison mismatch")
	}
	principal := Principal{csrfHash: storedHash}
	if !principal.ValidCSRF(token) || principal.ValidCSRF("wrong") {
		t.Fatal("principal CSRF validation mismatch")
	}
}
