//go:build mips || mipsle

package main

// The legacy update bridge still depends on SQLite. RUT906 updates use the
// middleware binary directly, so this target intentionally has no bridge.
func main() {}
