//go:build windows

package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestServiceMenuExitDoesNotStartService(t *testing.T) {
	var output bytes.Buffer
	err := serviceMenu(bufio.NewReader(strings.NewReader("0\n")), &output, serviceConfig{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Install / Update Service") {
		t.Fatalf("menu not rendered: %s", output.String())
	}
}

func TestServiceMenuRejectsInvalidSelection(t *testing.T) {
	var output bytes.Buffer
	err := serviceMenu(bufio.NewReader(strings.NewReader("x\n\n0\n")), &output, serviceConfig{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Invalid selection.") {
		t.Fatalf("invalid selection not reported: %s", output.String())
	}
}

func TestServiceMenuExitsOnClosedInput(t *testing.T) {
	err := serviceMenu(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, serviceConfig{}, "")
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}
