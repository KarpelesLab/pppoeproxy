package main

import (
	"fmt"
	"log"
	"net"

	"golang.org/x/sys/unix"
)

// SessionHandler handles PPPoE session packets
type SessionHandler struct {
	fd           int
	isServer     bool
	interfaceIdx int
}

// NewSessionHandler creates a new handler for PPPoE session packets
func NewSessionHandler(interfaceName string, isServer bool) (*SessionHandler, error) {
	// Get interface index
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("interface not found: %v", err)
	}

	// Create raw socket for PPPoE session packets
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(PPPoESession)))
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %v", err)
	}

	// Bind to the interface
	addr := unix.SockaddrLinklayer{
		Protocol: htons(PPPoESession),
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(fd, &addr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to bind socket: %v", err)
	}

	handler := &SessionHandler{
		fd:           fd,
		isServer:     isServer,
		interfaceIdx: iface.Index,
	}

	// Start packet processing
	go handler.processPackets()

	return handler, nil
}

// Close closes the socket
func (h *SessionHandler) Close() error {
	return unix.Close(h.fd)
}

// processPackets receives and processes PPPoE session packets
func (h *SessionHandler) processPackets() {
	buf := make([]byte, 2048)
	for {
		n, _, err := unix.Recvfrom(h.fd, buf, 0)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Printf("Error receiving packet: %v", err)
			return
		}

		if n < 20 { // Ethernet header (14) + minimum PPPoE header (6)
			continue
		}

		packet := buf[:n]
		h.handlePacket(packet)
	}
}

// handlePacket processes a PPPoE session packet
func (h *SessionHandler) handlePacket(packet []byte) {
	// Skip the Ethernet header (14 bytes)
	pppoeHeader := packet[14:]

	// Check if this is a PPPoE Session packet
	if pppoeHeader[0] != 0x11 { // PPPoE version 1, type 1
		return
	}

	// Forward the packet to the appropriate endpoint
	h.forwardPacket(packet)
}

// forwardPacket forwards the packet to the appropriate endpoint
func (h *SessionHandler) forwardPacket(packet []byte) {
	// In a real implementation, this would forward the packet to the client or server
	// based on the mode and the packet type
	log.Printf("Forwarding %d byte PPPoE session packet", len(packet))

	// This is a placeholder for the actual forwarding logic
	// The actual implementation would depend on how clients and servers communicate
	// For example, via TCP/IP or by injecting packets to another interface
}
