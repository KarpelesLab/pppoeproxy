package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"

	"golang.org/x/sys/unix"
)

// ForwardFunc is a function that forwards a packet
type ForwardFunc func(packet []byte)

// DiscoveryHandler handles PPPoE discovery packets
type DiscoveryHandler struct {
	fd           int
	isServer     bool
	interfaceIdx int
	forwardFunc  ForwardFunc
	mu           sync.Mutex
}

// NewDiscoveryHandler creates a new handler for PPPoE discovery packets
func NewDiscoveryHandler(interfaceName string, isServer bool) (*DiscoveryHandler, error) {
	// Get interface index
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("interface not found: %v", err)
	}

	// Create raw socket for PPPoE discovery packets
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(PPPoEDiscovery)))
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %v", err)
	}

	// Bind to the interface
	addr := unix.SockaddrLinklayer{
		Protocol: htons(PPPoEDiscovery),
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(fd, &addr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to bind socket: %v", err)
	}

	handler := &DiscoveryHandler{
		fd:           fd,
		isServer:     isServer,
		interfaceIdx: iface.Index,
	}

	// Start packet processing
	go handler.processPackets()

	return handler, nil
}

// Close closes the socket
func (h *DiscoveryHandler) Close() error {
	return unix.Close(h.fd)
}

// processPackets receives and processes PPPoE discovery packets
func (h *DiscoveryHandler) processPackets() {
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

// handlePacket processes a PPPoE discovery packet
func (h *DiscoveryHandler) handlePacket(packet []byte) {
	// Skip the Ethernet header (14 bytes)
	pppoeHeader := packet[14:]

	// Check if this is a PPPoE Discovery packet
	if pppoeHeader[0] != 0x11 { // PPPoE version 1, type 1
		return
	}

	// Extract packet type code
	code := pppoeHeader[1]

	// If we're in server mode and this is a PADI, handle Host-Uniq rewriting
	if h.isServer && code == PADI {
		h.rewriteHostUniq(packet)
	}

	// Forward the packet to the appropriate endpoint
	h.forwardPacket(packet)
}

// rewriteHostUniq rewrites the Host-Uniq tag in a PADI packet
func (h *DiscoveryHandler) rewriteHostUniq(packet []byte) {
	// PPPoE header is 6 bytes, so tags start at offset 20 (14 + 6)
	offset := 20

	// Process all tags
	for offset+4 <= len(packet) {
		tagType := binary.BigEndian.Uint16(packet[offset : offset+2])
		tagLen := binary.BigEndian.Uint16(packet[offset+2 : offset+4])

		if tagType == TagHostUniq && offset+4+int(tagLen) <= len(packet) {
			// Found Host-Uniq tag, rewrite it with simple XOR
			// This is just a placeholder; in a real implementation,
			// you might want a more sophisticated approach
			for i := 0; i < int(tagLen); i++ {
				packet[offset+4+i] ^= byte(0x42) // Simple XOR with a constant
			}
			return
		}

		// Move to next tag
		offset += 4 + int(tagLen)

		// End of tags
		if tagType == TagEndOfList {
			break
		}
	}
}

// forwardPacket forwards the packet to the appropriate endpoint
func (h *DiscoveryHandler) forwardPacket(packet []byte) {
	log.Printf("Forwarding %d byte PPPoE discovery packet", len(packet))

	// Call the registered forward function if available
	h.mu.Lock()
	forwardFunc := h.forwardFunc
	h.mu.Unlock()

	if forwardFunc != nil {
		forwardFunc(packet)
	}
}

// SetForwardFunc sets the function to be called when a packet needs to be forwarded
func (h *DiscoveryHandler) SetForwardFunc(f ForwardFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forwardFunc = f
}

// InjectPacket injects a packet into the interface
func (h *DiscoveryHandler) InjectPacket(packet []byte) {
	if len(packet) < 14 {
		log.Printf("Packet too short to inject: %d bytes", len(packet))
		return
	}

	// Prepare sockaddr for packet injection
	sa := unix.SockaddrLinklayer{
		Protocol: htons(PPPoEDiscovery),
		Ifindex:  h.interfaceIdx,
	}

	// Send packet to interface
	if err := unix.Sendto(h.fd, packet, 0, &sa); err != nil {
		log.Printf("Error injecting discovery packet: %v", err)
	} else {
		log.Printf("Injected %d byte PPPoE discovery packet", len(packet))
	}
}
