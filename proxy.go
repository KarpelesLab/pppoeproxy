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

// Client represents a network connection with synchronized access
type Client struct {
	conn       net.Conn
	writeMu    sync.Mutex // Mutex for connection writes
	remoteAddr string
}

// NewClient creates a new Client instance
func NewClient(conn net.Conn) *Client {
	return &Client{
		conn:       conn,
		remoteAddr: conn.RemoteAddr().String(),
	}
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// WritePacket writes a complete packet atomically
func (c *Client) WritePacket(packetType uint16, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// Write packet type
	if err := binary.Write(c.conn, binary.BigEndian, packetType); err != nil {
		return fmt.Errorf("error writing packet type: %v", err)
	}

	// Write length as varint
	length := len(data)
	for length >= 0x80 {
		if _, err := c.conn.Write([]byte{byte(length) | 0x80}); err != nil {
			return fmt.Errorf("error writing length: %v", err)
		}
		length >>= 7
	}
	if _, err := c.conn.Write([]byte{byte(length)}); err != nil {
		return fmt.Errorf("error writing length: %v", err)
	}

	// Write data
	if len(data) > 0 {
		if _, err := c.conn.Write(data); err != nil {
			return fmt.Errorf("error writing data: %v", err)
		}
	}

	return nil
}

// Proxy handles the client-server communication
type Proxy struct {
	isServer         bool
	address          string
	allowedIP        string
	discoveryHandler *DiscoveryHandler
	sessionHandler   *SessionHandler
	listener         net.Listener
	server           *Client
	clientsMu        sync.RWMutex
	clients          map[string]*Client
	closed           bool
	closedCh         chan struct{}
	serverMu         sync.Mutex   // Mutex for server connection access
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
		clients:          make(map[string]*Client),
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

	p.serverMu.Lock()
	if p.server != nil {
		p.server.Close()
		p.server = nil
	}
	p.serverMu.Unlock()

	p.clientsMu.Lock()
	for _, client := range p.clients {
		client.Close()
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

	p.serverMu.Lock()
	server := p.server
	p.serverMu.Unlock()

	if server == nil {
		return
	}

	// Send ping packet (type 0, empty data)
	if err := server.WritePacket(PacketTypePing, []byte{}); err != nil {
		log.Printf("Error sending ping: %v", err)
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

		client := NewClient(conn)
		log.Printf("Accepted connection from %s", clientIP)
		p.clientsMu.Lock()
		p.clients[client.remoteAddr] = client
		p.clientsMu.Unlock()

		go p.handleClient(client)
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
func (p *Proxy) handleClient(client *Client) {
	defer func() {
		client.Close()
		p.clientsMu.Lock()
		delete(p.clients, client.remoteAddr)
		p.clientsMu.Unlock()
		log.Printf("Client %s disconnected", client.remoteAddr)
	}()

	buffer := make([]byte, 4096)
	for {
		// Read packet type (uint16)
		var packetType uint16
		if err := binary.Read(client.conn, binary.BigEndian, &packetType); err != nil {
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

			_, err := client.conn.Read(buffer[:1])
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
			if err := client.WritePacket(PacketTypePong, []byte{}); err != nil {
				log.Printf("Error sending pong: %v", err)
				return
			}
			log.Printf("Received ping from client %s, sent pong", client.remoteAddr)

		case PacketTypePong:
			// Just log receipt of pong
			log.Printf("Received pong from client %s", client.remoteAddr)

		case PacketTypeDiscovery, PacketTypeSession:
			// Resize buffer if needed
			if length > uint64(len(buffer)) {
				newSize := length
				if newSize > 65536 {
					log.Printf("Packet too large: %d bytes", length)
					return
				}
				buffer = make([]byte, newSize)
			}

			// Read packet data for non-zero length packets
			if length > 0 {
				if _, err := io.ReadFull(client.conn, buffer[:length]); err != nil {
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
			log.Printf("Unknown packet type from client %s: %d", client.remoteAddr, packetType)
			// Skip any data associated with an unknown packet type
			if length > 0 {
				if length > 1048576 { // 1MB limit for skipping
					log.Printf("Unknown packet too large to skip: %d bytes", length)
					return
				}

				if _, err := io.CopyN(io.Discard, client.conn, int64(length)); err != nil {
					log.Printf("Error skipping unknown packet data: %v", err)
					return
				}
			}
		}
	}
}

// connectToServer connects to the remote server
func (p *Proxy) connectToServer() error {
	p.serverMu.Lock()
	defer p.serverMu.Unlock()

	// Close existing connection if any
	if p.server != nil {
		p.server.Close()
		p.server = nil
	}

	conn, err := net.Dial("tcp", p.address)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}

	p.server = NewClient(conn)
	log.Printf("Connected to server at %s", p.address)
	go p.handleServerConnection(p.server)
	return nil
}

// handleServerConnection processes packets from the server
func (p *Proxy) handleServerConnection(client *Client) {
	defer func() {
		p.serverMu.Lock()
		if p.server == client {
			p.server = nil
		}
		p.serverMu.Unlock()

		client.Close()
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
		if err := binary.Read(client.conn, binary.BigEndian, &packetType); err != nil {
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

			_, err := client.conn.Read(buffer[:1])
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
			if err := client.WritePacket(PacketTypePong, []byte{}); err != nil {
				log.Printf("Error sending pong: %v", err)
				return
			}
			log.Printf("Received ping, sent pong")

		case PacketTypePong:
			// Just log receipt of pong
			log.Printf("Received pong from server")

		case PacketTypeDiscovery, PacketTypeSession:
			// Resize buffer if needed
			if length > uint64(len(buffer)) {
				newSize := length
				if newSize > 65536 {
					log.Printf("Packet from server too large: %d bytes", length)
					return
				}
				buffer = make([]byte, newSize)
			}

			// Read packet data for non-zero length packets
			if length > 0 {
				if _, err := io.ReadFull(client.conn, buffer[:length]); err != nil {
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
				if length > 1048576 { // 1MB limit for skipping
					log.Printf("Unknown packet too large to skip: %d bytes", length)
					return
				}

				if _, err := io.CopyN(io.Discard, client.conn, int64(length)); err != nil {
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

		// Broadcast to all clients
		for _, client := range p.clients {
			if err := client.WritePacket(PacketTypeDiscovery, packet); err != nil {
				log.Printf("Error sending discovery packet to client %s: %v", client.remoteAddr, err)
			}
		}
	} else {
		// In client mode, send to server
		p.serverMu.Lock()
		server := p.server
		p.serverMu.Unlock()

		if server == nil {
			return
		}

		// Send to server
		if err := server.WritePacket(PacketTypeDiscovery, packet); err != nil {
			log.Printf("Error sending discovery packet to server: %v", err)
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

		// Broadcast to all clients
		for _, client := range p.clients {
			if err := client.WritePacket(PacketTypeSession, packet); err != nil {
				log.Printf("Error sending session packet to client %s: %v", client.remoteAddr, err)
			}
		}
	} else {
		// In client mode, send to server
		p.serverMu.Lock()
		server := p.server
		p.serverMu.Unlock()

		if server == nil {
			return
		}

		// Send to server
		if err := server.WritePacket(PacketTypeSession, packet); err != nil {
			log.Printf("Error sending session packet to server: %v", err)
		}
	}
}
