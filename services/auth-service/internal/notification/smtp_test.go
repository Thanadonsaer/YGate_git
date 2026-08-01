package notification

import "testing"

func TestNewSMTPResetNotifierRequiresSecureInputs(t *testing.T) {
	for _, test := range []struct {
		addr, from, resetURL string
	}{
		{addr: "missing-port", from: "scada@example.com", resetURL: "https://scada.example.com/reset"},
		{addr: "smtp.example.com:587", from: "bad\r\nBcc: victim@example.com", resetURL: "https://scada.example.com/reset"},
		{addr: "smtp.example.com:587", from: "scada@example.com", resetURL: "http://scada.example.com/reset"},
	} {
		if _, err := NewSMTPResetNotifier(test.addr, test.from, "", "", test.resetURL); err == nil {
			t.Fatalf("expected invalid config: %+v", test)
		}
	}
	if _, err := NewSMTPResetNotifier("smtp.example.com:587", "scada@example.com", "", "", "https://scada.example.com/reset"); err != nil {
		t.Fatal(err)
	}
}
