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

// PPPoE Discovery Tag types
const (
	TagEndOfList    = 0x0000
	TagServiceName  = 0x0101
	TagACName       = 0x0102
	TagHostUniq     = 0x0103
	TagACCookie     = 0x0104
	TagVendorSpec   = 0x0105
	TagRelaySession = 0x0110
	TagServiceName2 = 0x0201
	TagACName2      = 0x0203
	TagGenericError = 0x0203
)

// PPPoE Packet types
const (
	PADI = 0x09 // PPPoE Active Discovery Initiation
	PADO = 0x07 // PPPoE Active Discovery Offer
	PADR = 0x19 // PPPoE Active Discovery Request
	PADS = 0x65 // PPPoE Active Discovery Session-confirmation
	PADT = 0xa7 // PPPoE Active Discovery Terminate
)
