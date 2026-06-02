//go:build windows

package main

func lockFile(lockPath string) (func(), error) {
	return func() {}, nil
}
