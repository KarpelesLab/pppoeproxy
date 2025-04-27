package main

import (
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

	// Get packet type
	code := pppoeHeader[1]

	// Log the packet with appropriate type description
	var packetType string
	switch code {
	case PADI:
		packetType = "PADI (Discovery Initiation)"
	case PADO:
		packetType = "PADO (Discovery Offer)"
	case PADR:
		packetType = "PADR (Discovery Request)"
	case PADS:
		packetType = "PADS (Discovery Session-confirmation)"
	case PADT:
		packetType = "PADT (Discovery Terminate)"
	default:
		packetType = fmt.Sprintf("Unknown (0x%02x)", code)
	}

	log.Printf("PPPoE Discovery packet received: %s, %d bytes", packetType, len(packet))

	// Forward the packet to the appropriate endpoint
	h.forwardPacket(packet)
}

// forwardPacket forwards the packet to the appropriate endpoint
func (h *DiscoveryHandler) forwardPacket(packet []byte) {
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

	// Check the packet type
	if len(packet) >= 15 { // 14 bytes Ethernet header + at least 1 byte for code
		code := packet[15] // PPPoE packet type code at offset 15

		var packetType string
		switch code {
		case PADI:
			packetType = "PADI (Discovery Initiation)"
		case PADO:
			packetType = "PADO (Discovery Offer)"
		case PADR:
			packetType = "PADR (Discovery Request)"
		case PADS:
			packetType = "PADS (Discovery Session-confirmation)"
		case PADT:
			packetType = "PADT (Discovery Terminate)"
		default:
			packetType = fmt.Sprintf("Unknown (0x%02x)", code)
		}

		// Send packet to interface
		if err := unix.Sendto(h.fd, packet, 0, &sa); err != nil {
			log.Printf("Error injecting discovery packet (%s): %v", packetType, err)
		} else {
			log.Printf("Injected %s PPPoE discovery packet, %d bytes", packetType, len(packet))
		}
	} else {
		// Send packet to interface (malformed packet case)
		if err := unix.Sendto(h.fd, packet, 0, &sa); err != nil {
			log.Printf("Error injecting malformed discovery packet: %v", err)
		} else {
			log.Printf("Injected malformed PPPoE discovery packet, %d bytes", len(packet))
		}
	}
}
