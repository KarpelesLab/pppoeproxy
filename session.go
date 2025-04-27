package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"

	"golang.org/x/sys/unix"
)

// SessionHandler handles PPPoE session packets
type SessionHandler struct {
	fd           int
	isServer     bool
	interfaceIdx int
	forwardFunc  ForwardFunc
	mu           sync.Mutex
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

	// Extract the session ID
	sessionID := binary.BigEndian.Uint16(pppoeHeader[2:4])

	// Check if this is a session establishment or termination
	if len(pppoeHeader) >= 8 { // PPPoE header (6) + at least protocol (2)
		// Get the PPP protocol type
		protocol := binary.BigEndian.Uint16(pppoeHeader[6:8])

		// Log session establishment (LCP) or termination
		if protocol == 0xc021 { // LCP protocol
			if len(pppoeHeader) >= 9 { // Additional byte for LCP code
				lcpCode := pppoeHeader[8]
				if lcpCode == 1 { // Configure-Request
					log.Printf("PPPoE Session establishment request, ID: 0x%04x", sessionID)
				} else if lcpCode == 9 { // Echo-Request (keepalive)
					// Don't log normal keepalives
				} else if lcpCode == 5 { // Terminate-Request
					log.Printf("PPPoE Session termination request, ID: 0x%04x", sessionID)
				}
			}
		} else if protocol == 0x0021 { // IP protocol (established session)
			// Don't log data packets
		} else {
			// Log other protocol packets (like authentication)
			if protocol != 0 { // Avoid logging padding
				log.Printf("PPPoE Session packet, ID: 0x%04x, Protocol: 0x%04x", sessionID, protocol)
			}
		}
	}

	// Forward the packet to the appropriate endpoint
	h.forwardPacket(packet)
}

// forwardPacket forwards the packet to the appropriate endpoint
func (h *SessionHandler) forwardPacket(packet []byte) {
	// Call the registered forward function if available
	h.mu.Lock()
	forwardFunc := h.forwardFunc
	h.mu.Unlock()

	if forwardFunc != nil {
		forwardFunc(packet)
	}
}

// SetForwardFunc sets the function to be called when a packet needs to be forwarded
func (h *SessionHandler) SetForwardFunc(f ForwardFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forwardFunc = f
}

// InjectPacket injects a packet into the interface
func (h *SessionHandler) InjectPacket(packet []byte) {
	if len(packet) < 14 {
		log.Printf("Packet too short to inject: %d bytes", len(packet))
		return
	}

	// Prepare sockaddr for packet injection
	sa := unix.SockaddrLinklayer{
		Protocol: htons(PPPoESession),
		Ifindex:  h.interfaceIdx,
	}

	// Extract session information for logging
	if len(packet) >= 20 { // 14 + 6 (Ethernet + PPPoE headers)
		// Get session ID
		sessionID := binary.BigEndian.Uint16(packet[16:18])

		// Check for LCP packets (session establishment/termination)
		if len(packet) >= 22 { // + 2 for protocol
			protocol := binary.BigEndian.Uint16(packet[20:22])

			if protocol == 0xc021 && len(packet) >= 23 { // LCP protocol
				lcpCode := packet[22]
				if lcpCode == 1 { // Configure-Request
					log.Printf("Injecting PPPoE Session establishment request, ID: 0x%04x", sessionID)
				} else if lcpCode == 5 { // Terminate-Request
					log.Printf("Injecting PPPoE Session termination request, ID: 0x%04x", sessionID)
				}
			}
		}
	}

	// Send packet to interface (don't log regular data packets)
	if err := unix.Sendto(h.fd, packet, 0, &sa); err != nil {
		log.Printf("Error injecting session packet: %v", err)
	}
}
