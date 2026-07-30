package web

import (
	"regexp"
	"strings"
	"testing"
)

func TestDeviceDialogIncludesRequiredModelSelect(t *testing.T) {
	page, err := files.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	visibleHTML := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(string(page), "")
	for _, required := range []string{
		`id="connection-form"`,
		`id="connection-set" required`,
		`$("connection-set").value`,
	} {
		if !strings.Contains(visibleHTML, required) {
			t.Fatalf("device dialog missing active markup %q", required)
		}
	}
}
