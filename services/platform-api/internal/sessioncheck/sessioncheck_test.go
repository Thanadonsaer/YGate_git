package sessioncheck

import "testing"

func TestValidCSRF(t *testing.T) {
	token := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	hash, err := hashPresentedToken(token)
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{csrfHash: hash}
	if !p.ValidCSRF(token) {
		t.Fatal("expected matching token to validate")
	}
	if p.ValidCSRF("wrong-token") {
		t.Fatal("expected mismatched token to fail validation")
	}
	if p.ValidCSRF("") {
		t.Fatal("expected empty token to fail validation")
	}
}
