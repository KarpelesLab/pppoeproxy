package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Proxy handles the client-server communication
type Proxy struct {
	isServer         bool
	address          string
	allowedIP        string
	discoveryHandler *DiscoveryHandler
	sessionHandler   *SessionHandler
	listener         net.Listener
	conn             net.Conn
	clientsMu        sync.RWMutex
	clients          map[string]net.Conn
	closed           bool
	closedCh         chan struct{}
	connMu           sync.Mutex   // Mutex for connection access
	reconnectTimer   *time.Timer  // Timer for reconnection attempts
	pingTicker       *time.Ticker // Ticker for sending pings
}

// NewProxy creates a new proxy instance
func NewProxy(isServer bool, address, allowedIP string, discoveryHandler *DiscoveryHandler, sessionHandler *SessionHandler) (*Proxy, error) {
	p := &Proxy{
		isServer:         isServer,
		address:          address,
		allowedIP:        allowedIP,
		discoveryHandler: discoveryHandler,
		sessionHandler:   sessionHandler,
		clients:          make(map[string]net.Conn),
		closedCh:         make(chan struct{}),
	}

	// Set the packet handlers
	discoveryHandler.SetForwardFunc(p.handleDiscoveryPacket)
	sessionHandler.SetForwardFunc(p.handleSessionPacket)

	// Start server or connect to server
	if isServer {
		if err := p.startServer(); err != nil {
			return nil, err
		}
	} else {
		// In client mode, set up ping ticker and connect
		p.pingTicker = time.NewTicker(60 * time.Second)
		go p.pingLoop()

		if err := p.connectToServer(); err != nil {
			log.Printf("Initial connection failed: %v", err)
			// Start reconnection attempts
			p.scheduleReconnect()
		}
	}

	return p, nil
}

// Close shuts down the proxy
func (p *Proxy) Close() error {
	p.closed = true
	close(p.closedCh)

	if p.listener != nil {
		p.listener.Close()
	}

	p.connMu.Lock()
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
	p.connMu.Unlock()

	p.clientsMu.Lock()
	for _, conn := range p.clients {
		conn.Close()
	}
	p.clientsMu.Unlock()

	// Stop timers and tickers
	if p.reconnectTimer != nil {
		p.reconnectTimer.Stop()
	}

	if p.pingTicker != nil {
		p.pingTicker.Stop()
	}

	return nil
}

// pingLoop sends periodic pings to the server
func (p *Proxy) pingLoop() {
	for {
		select {
		case <-p.closedCh:
			return
		case <-p.pingTicker.C:
			p.sendPing()
		}
	}
}

// sendPing sends a ping packet to the server
func (p *Proxy) sendPing() {
	if p.closed {
		return
	}

	p.connMu.Lock()
	conn := p.conn
	p.connMu.Unlock()

	if conn == nil {
		return
	}

	// Send ping packet (type 0, zero length)
	if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypePing)); err != nil {
		log.Printf("Error sending ping: %v", err)
		return
	}

	// Send zero length (single byte 0)
	if _, err := conn.Write([]byte{0}); err != nil {
		log.Printf("Error sending ping length: %v", err)
		return
	}

	log.Printf("Sent ping to server")
}

// scheduleReconnect schedules a reconnection attempt
func (p *Proxy) scheduleReconnect() {
	if p.closed {
		return
	}

	// Stop any existing timer
	if p.reconnectTimer != nil {
		p.reconnectTimer.Stop()
	}

	// Reconnect after 5 seconds
	p.reconnectTimer = time.AfterFunc(5*time.Second, func() {
		if p.closed {
			return
		}

		log.Printf("Attempting to reconnect to server...")
		err := p.connectToServer()
		if err != nil {
			log.Printf("Reconnection failed: %v", err)
			p.scheduleReconnect()
		} else {
			log.Printf("Successfully reconnected to server")
		}
	})
}

// startServer starts a TCP server to accept client connections
func (p *Proxy) startServer() error {
	var err error
	p.listener, err = net.Listen("tcp", p.address)
	if err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	go p.acceptClients()
	log.Printf("Server listening on %s", p.address)
	return nil
}

// acceptClients accepts and handles client connections
func (p *Proxy) acceptClients() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			if p.closed {
				return
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		clientIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

		// Check if client IP is allowed
		if !p.isClientAllowed(clientIP) {
			log.Printf("Rejected connection from unauthorized client: %s", clientIP)
			conn.Close()
			continue
		}

		log.Printf("Accepted connection from %s", clientIP)
		p.clientsMu.Lock()
		p.clients[conn.RemoteAddr().String()] = conn
		p.clientsMu.Unlock()

		go p.handleClient(conn)
	}
}

// isClientAllowed checks if the client IP is allowed to connect
func (p *Proxy) isClientAllowed(clientIP string) bool {
	// Extract the IP part without the port
	ipParts := strings.Split(clientIP, ":")
	ip := ipParts[0]

	// Check if it matches the allowed IP
	return ip == p.allowedIP
}

// handleClient processes packets from a connected client
func (p *Proxy) handleClient(conn net.Conn) {
	defer func() {
		conn.Close()
		p.clientsMu.Lock()
		delete(p.clients, conn.RemoteAddr().String())
		p.clientsMu.Unlock()
		log.Printf("Client %s disconnected", conn.RemoteAddr())
	}()

	buffer := make([]byte, 4096)
	for {
		// Read packet type (uint16)
		var packetType uint16
		if err := binary.Read(conn, binary.BigEndian, &packetType); err != nil {
			if err == io.EOF {
				return
			}
			log.Printf("Error reading packet type: %v", err)
			return
		}

		// Read length (varint)
		var length uint64
		var shift uint
		for {
			if len(buffer) == 0 {
				log.Printf("Buffer too small for varint")
				return
			}

			_, err := conn.Read(buffer[:1])
			if err != nil {
				log.Printf("Error reading varint: %v", err)
				return
			}

			b := buffer[0]
			length |= uint64(b&0x7f) << shift
			shift += 7

			if b&0x80 == 0 {
				break
			}

			if shift > 63 {
				log.Printf("Varint too large")
				return
			}
		}

		// Process packet based on type
		switch packetType {
		case PacketTypePing:
			// Respond with pong
			if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypePong)); err != nil {
				log.Printf("Error sending pong: %v", err)
				return
			}
			if _, err := conn.Write([]byte{0}); err != nil { // Zero length
				log.Printf("Error sending pong length: %v", err)
				return
			}
			log.Printf("Received ping from client %s, sent pong", conn.RemoteAddr())

		case PacketTypePong:
			// Just log receipt of pong
			log.Printf("Received pong from client %s", conn.RemoteAddr())

		case PacketTypeDiscovery, PacketTypeSession:
			if length > uint64(len(buffer)) {
				log.Printf("Packet too large: %d bytes", length)
				return
			}

			// Read packet data for non-zero length packets
			if length > 0 {
				if _, err := io.ReadFull(conn, buffer[:length]); err != nil {
					log.Printf("Error reading packet data: %v", err)
					return
				}
			}

			// Process packet based on type
			if packetType == PacketTypeDiscovery {
				// Inject the packet into the interface
				p.discoveryHandler.InjectPacket(buffer[:length])
			} else {
				// Inject the packet into the interface
				p.sessionHandler.InjectPacket(buffer[:length])
			}

		default:
			log.Printf("Unknown packet type from client %s: %d", conn.RemoteAddr(), packetType)
			// Skip any data associated with an unknown packet type
			if length > 0 {
				if _, err := io.ReadFull(conn, buffer[:length]); err != nil {
					log.Printf("Error skipping unknown packet data: %v", err)
					return
				}
			}
		}
	}
}

// connectToServer connects to the remote server
func (p *Proxy) connectToServer() error {
	p.connMu.Lock()
	defer p.connMu.Unlock()

	// Close existing connection if any
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}

	var err error
	conn, err := net.Dial("tcp", p.address)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}

	p.conn = conn
	log.Printf("Connected to server at %s", p.address)
	go p.handleServerConnection()
	return nil
}

// handleServerConnection processes packets from the server
func (p *Proxy) handleServerConnection() {
	conn := p.conn // Local reference to the connection

	defer func() {
		p.connMu.Lock()
		if p.conn == conn {
			p.conn = nil
		}
		p.connMu.Unlock()

		conn.Close()
		log.Printf("Disconnected from server")

		// Schedule reconnection if we're not closing
		if !p.closed {
			p.scheduleReconnect()
		}
	}()

	buffer := make([]byte, 4096)
	for {
		// Read packet type (uint16)
		var packetType uint16
		if err := binary.Read(conn, binary.BigEndian, &packetType); err != nil {
			if err == io.EOF || p.closed {
				return
			}
			log.Printf("Error reading packet type from server: %v", err)
			return
		}

		// Read length (varint)
		var length uint64
		var shift uint
		for {
			if len(buffer) == 0 {
				log.Printf("Buffer too small for varint")
				return
			}

			_, err := conn.Read(buffer[:1])
			if err != nil {
				log.Printf("Error reading varint from server: %v", err)
				return
			}

			b := buffer[0]
			length |= uint64(b&0x7f) << shift
			shift += 7

			if b&0x80 == 0 {
				break
			}

			if shift > 63 {
				log.Printf("Varint too large")
				return
			}
		}

		// Process packet based on type
		switch packetType {
		case PacketTypePing:
			// Respond with pong
			if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypePong)); err != nil {
				log.Printf("Error sending pong: %v", err)
				return
			}
			if _, err := conn.Write([]byte{0}); err != nil { // Zero length
				log.Printf("Error sending pong length: %v", err)
				return
			}
			log.Printf("Received ping, sent pong")

		case PacketTypePong:
			// Just log receipt of pong
			log.Printf("Received pong from server")

		case PacketTypeDiscovery, PacketTypeSession:
			if length > uint64(len(buffer)) {
				log.Printf("Packet from server too large: %d bytes", length)
				return
			}

			// Read packet data for non-zero length packets
			if length > 0 {
				if _, err := io.ReadFull(conn, buffer[:length]); err != nil {
					log.Printf("Error reading packet data from server: %v", err)
					return
				}
			}

			// Process packet based on type
			if packetType == PacketTypeDiscovery {
				// Inject the packet into the interface
				p.discoveryHandler.InjectPacket(buffer[:length])
			} else {
				// Inject the packet into the interface
				p.sessionHandler.InjectPacket(buffer[:length])
			}

		default:
			log.Printf("Unknown packet type from server: %d", packetType)
			// Skip any data associated with an unknown packet type
			if length > 0 {
				if _, err := io.ReadFull(conn, buffer[:length]); err != nil {
					log.Printf("Error skipping unknown packet data: %v", err)
					return
				}
			}
		}
	}
}

// handleDiscoveryPacket sends a discovery packet to the server or clients
func (p *Proxy) handleDiscoveryPacket(packet []byte) {
	if p.closed {
		return
	}

	if p.isServer {
		// In server mode, broadcast to all clients
		p.clientsMu.RLock()
		defer p.clientsMu.RUnlock()

		if len(p.clients) == 0 {
			return
		}

		// Prepare packet header
		for _, conn := range p.clients {
			// Send packet type
			if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypeDiscovery)); err != nil {
				log.Printf("Error sending packet type: %v", err)
				continue
			}

			// Send varint length
			length := len(packet)
			for length >= 0x80 {
				if _, err := conn.Write([]byte{byte(length) | 0x80}); err != nil {
					log.Printf("Error sending length: %v", err)
					continue
				}
				length >>= 7
			}
			if _, err := conn.Write([]byte{byte(length)}); err != nil {
				log.Printf("Error sending length: %v", err)
				continue
			}

			// Send packet data
			if _, err := conn.Write(packet); err != nil {
				log.Printf("Error sending packet data: %v", err)
				continue
			}
		}
	} else {
		// In client mode, send to server
		p.connMu.Lock()
		conn := p.conn
		p.connMu.Unlock()

		if conn == nil {
			return
		}

		// Send packet type
		if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypeDiscovery)); err != nil {
			log.Printf("Error sending packet type to server: %v", err)
			return
		}

		// Send varint length
		length := len(packet)
		for length >= 0x80 {
			if _, err := conn.Write([]byte{byte(length) | 0x80}); err != nil {
				log.Printf("Error sending length to server: %v", err)
				return
			}
			length >>= 7
		}
		if _, err := conn.Write([]byte{byte(length)}); err != nil {
			log.Printf("Error sending length to server: %v", err)
			return
		}

		// Send packet data
		if _, err := conn.Write(packet); err != nil {
			log.Printf("Error sending packet data to server: %v", err)
			return
		}
	}
}

// handleSessionPacket sends a session packet to the server or clients
func (p *Proxy) handleSessionPacket(packet []byte) {
	if p.closed {
		return
	}

	if p.isServer {
		// In server mode, broadcast to all clients
		p.clientsMu.RLock()
		defer p.clientsMu.RUnlock()

		if len(p.clients) == 0 {
			return
		}

		// Prepare packet header
		for _, conn := range p.clients {
			// Send packet type
			if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypeSession)); err != nil {
				log.Printf("Error sending packet type: %v", err)
				continue
			}

			// Send varint length
			length := len(packet)
			for length >= 0x80 {
				if _, err := conn.Write([]byte{byte(length) | 0x80}); err != nil {
					log.Printf("Error sending length: %v", err)
					continue
				}
				length >>= 7
			}
			if _, err := conn.Write([]byte{byte(length)}); err != nil {
				log.Printf("Error sending length: %v", err)
				continue
			}

			// Send packet data
			if _, err := conn.Write(packet); err != nil {
				log.Printf("Error sending packet data: %v", err)
				continue
			}
		}
	} else {
		// In client mode, send to server
		p.connMu.Lock()
		conn := p.conn
		p.connMu.Unlock()

		if conn == nil {
			return
		}

		// Send packet type
		if err := binary.Write(conn, binary.BigEndian, uint16(PacketTypeSession)); err != nil {
			log.Printf("Error sending packet type to server: %v", err)
			return
		}

		// Send varint length
		length := len(packet)
		for length >= 0x80 {
			if _, err := conn.Write([]byte{byte(length) | 0x80}); err != nil {
				log.Printf("Error sending length to server: %v", err)
				return
			}
			length >>= 7
		}
		if _, err := conn.Write([]byte{byte(length)}); err != nil {
			log.Printf("Error sending length to server: %v", err)
			return
		}

		// Send packet data
		if _, err := conn.Write(packet); err != nil {
			log.Printf("Error sending packet data to server: %v", err)
			return
		}
	}
}
