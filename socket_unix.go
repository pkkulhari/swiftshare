//go:build !windows

package main

import "syscall"

func setSocketOpts(fd uintptr) error {
	// Enable TCP keep-alive and set buffer sizes
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, BUFFER_SIZE)
	syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, BUFFER_SIZE)
	return nil
}
