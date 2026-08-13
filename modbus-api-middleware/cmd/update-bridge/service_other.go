//go:build !windows

package main

func runMaybeService(string) (bool, error) { return false, nil }
