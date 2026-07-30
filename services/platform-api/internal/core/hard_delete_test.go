package core

import "testing"

func TestValidHardDeleteConfirmation(t *testing.T) {
	if !validHardDeleteConfirmation("DELETE", "DELETE PLANT PLANT-01") || !validHardDeleteConfirmation("DELETE PLANT PLANT-01", "DELETE PLANT PLANT-01") {
		t.Fatal("supported hard-delete confirmation rejected")
	}
	if validHardDeleteConfirmation("delete", "DELETE PLANT PLANT-01") || validHardDeleteConfirmation("DELETE PLANT OTHER", "DELETE PLANT PLANT-01") {
		t.Fatal("unsafe hard-delete confirmation accepted")
	}
}
