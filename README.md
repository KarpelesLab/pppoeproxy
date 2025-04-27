# PPPoE Proxy

A proxy system for PPPoE (Point-to-Point Protocol over Ethernet) connections written in Go.

## Overview

PPPoE Proxy allows tunneling PPPoE connections between networks by proxying both discovery and session packets. This enables:

- Extending PPPoE connectivity across networks
- Controlling and monitoring PPPoE traffic
- Adding authentication layers to PPPoE connections

The system operates in either client or server mode:

- **Client Mode**: Captures PPPoE traffic from a local interface and forwards it to a remote server
- **Server Mode**: Receives traffic from clients and forwards it to the actual PPPoE server, with authentication

## Features

- Proxying of PPPoE Discovery packets (0x8863)
- Proxying of PPPoE Session packets (0x8864)
- Host-Uniq tag rewriting for secure client-server authentication
- Raw socket handling for efficient packet capture and injection
- IP-based access control for client connections
- Automatic reconnection for client mode
- Ping/pong keepalive mechanism (60-second interval)
- Thread-safe connection handling

## Usage

```
# Server Mode
./pppoeproxy -interface eth0 -mode server -address 0.0.0.0:8000 -allow 192.168.1.2

# Client Mode
./pppoeproxy -interface eth0 -mode client -address 192.168.1.1:8000
```

### Command Line Options

- `-interface`: Network interface to capture and inject PPPoE packets (required)
- `-mode`: Operation mode, either "client" or "server" (default: "client")
- `-address`: Address to connect to (client mode) or listen on (server mode) (required)
- `-allow`: IP address allowed to connect (server mode only, default: "127.0.0.1")

## How It Works

1. **PPPoE Discovery Phase**:
   - In client mode, captures PADI, PADO, PADR, and PADS packets
   - In server mode, captures packets and rewrites Host-Uniq tags with a value derived from the shared secret
   - Forwards packets between the client, server, and the actual PPPoE server

2. **PPPoE Session Phase**:
   - Captures and forwards session packets to maintain the tunnel
   - Preserves PPPoE session IDs and packet integrity

## Building

```
make
```

## Requirements

- Go 1.20 or higher
- Linux system with root access (for raw socket operations)
- Administrative privileges on network interfaces