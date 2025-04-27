package main

// Ethernet protocol types
const (
	PPPoEDiscovery = 0x8863 // PPPoE Discovery stage
	PPPoESession   = 0x8864 // PPPoE Session stage
)

// Protocol packet types
const (
	PacketTypePing      = 0 // Ping packet for keepalive
	PacketTypePong      = 1 // Pong response to ping
	PacketTypeDiscovery = 2 // Discovery packet type for tunnel
	PacketTypeSession   = 3 // Session packet type for tunnel
)

// PPPoE Packet types
const (
	PADI = 0x09 // PPPoE Active Discovery Initiation
	PADO = 0x07 // PPPoE Active Discovery Offer
	PADR = 0x19 // PPPoE Active Discovery Request
	PADS = 0x65 // PPPoE Active Discovery Session-confirmation
	PADT = 0xa7 // PPPoE Active Discovery Terminate
)