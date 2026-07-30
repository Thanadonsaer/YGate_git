//go:build !windows

package main

func runInteractiveServiceMenu(publicKey string) (bool, error) {
	return false, nil
}
