//go:build windows

package main

import "golang.org/x/sys/windows"

func setSocketOpts(fd uintptr) error {
	h := windows.Handle(fd)
	windows.SetsockoptInt(h, windows.SOL_SOCKET, windows.SO_KEEPALIVE, 1)
	windows.SetsockoptInt(h, windows.SOL_SOCKET, windows.SO_RCVBUF, BUFFER_SIZE)
	windows.SetsockoptInt(h, windows.SOL_SOCKET, windows.SO_SNDBUF, BUFFER_SIZE)
	return nil
}
