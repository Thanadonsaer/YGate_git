//go:build !windows

package main

import "context"

func runMaybeService(run func(context.Context) error) (bool, error) {
	return false, nil
}
