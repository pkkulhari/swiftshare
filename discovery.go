package main

import (
	"context"
	"fmt"

	"github.com/grandcat/zeroconf"
)

const (
	serviceName = "_swiftshare._tcp"
	domain      = "local."
	port        = 3010
)

type Discovery struct {
	server   *zeroconf.Server
	resolver *zeroconf.Resolver
	entries  chan *zeroconf.ServiceEntry
}

func NewDiscovery() *Discovery {
	return &Discovery{
		entries: make(chan *zeroconf.ServiceEntry),
	}
}

// RegisterService registers this instance as a SwiftShare service
func (d *Discovery) RegisterService(name string) error {
	var err error
	d.server, err = zeroconf.Register(
		name,               // Instance name
		serviceName,        // Service type
		domain,             // Domain
		port,               // Port
		[]string{"txtv=0"}, // Metadata
		nil,                // Interface list (nil = all)
	)
	if err != nil {
		return fmt.Errorf("failed to register service: %v", err)
	}
	return nil
}

// StartDiscovery starts discovering other SwiftShare instances on the network
func (d *Discovery) StartDiscovery(ctx context.Context) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("failed to create resolver: %v", err)
	}
	d.resolver = resolver

	// Start listening for new services
	err = resolver.Browse(ctx, serviceName, domain, d.entries)
	if err != nil {
		return fmt.Errorf("failed to browse services: %v", err)
	}

	return nil
}

// GetEntries returns the channel of discovered services
func (d *Discovery) GetEntries() chan *zeroconf.ServiceEntry {
	return d.entries
}

// GetIPAddress returns the first non-loopback IPv4 address of the service
func (d *Discovery) GetIPAddress(entry *zeroconf.ServiceEntry) string {
	for _, addr := range entry.AddrIPv4 {
		if !addr.IsLoopback() {
			return addr.String()
		}
	}
	return ""
}

// Shutdown stops the service discovery and registration
func (d *Discovery) Shutdown() {
	if d.server != nil {
		d.server.Shutdown()
	}
	close(d.entries)
}
