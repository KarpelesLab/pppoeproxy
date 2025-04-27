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
   - In server mode, captures and forwards packets to connected clients
   - Forwards packets between the client, server, and the actual PPPoE server

2. **PPPoE Session Phase**:
   - Captures and forwards session packets to maintain the tunnel
   - Preserves PPPoE session IDs and packet integrity

## Use Case: NTT Lines in Japan

In Japan, NTT allows up to 2 PPPoE sessions on a single line. This enables an interesting use case:

1. Set up a macvlan interface on your primary internet-connected device
2. Run this proxy in server mode on that macvlan interface
3. Run a client on a remote device (e.g., in another location)
4. The remote device can now establish its own PPPoE session through your NTT line

This effectively allows you to share your NTT connection with a remote location while maintaining separate PPPoE sessions, each with its own public IP address.

### Setting Up the Macvlan Interface

On your primary device that's connected to the NTT line, create a macvlan interface:

```bash
# Create a macvlan interface attached to your physical interface (e.g., eth0)
sudo ip link add link eth0 name pppoe-proxy type macvlan mode bridge

# Set a unique MAC address for this interface
sudo ip link set dev pppoe-proxy address 00:11:22:33:44:55

# Bring the interface up
sudo ip link set dev pppoe-proxy up
```

### Running the Server

On the same primary device, run the PPPoE proxy in server mode:

```bash
# Run the server on the macvlan interface, listening on port 8000
sudo ./pppoeproxy -interface pppoe-proxy -mode server -address 0.0.0.0:8000 -allow 192.168.1.2
```

Replace `192.168.1.2` with the IP address of your remote client device.

### Running the Client

On the remote device:

```bash
# Run the client connecting to the server's IP address
sudo ./pppoeproxy -interface eth0 -mode client -address 192.168.1.1:8000
```

Replace `eth0` with your network interface and `192.168.1.1` with the IP address of your server.

### Setting Up the PPPoE Client

On the remote device, configure your PPPoE client software to connect through the proxy:

```bash
# Example using pppd for Linux
sudo pppd plugin rp-pppoe.so eth0 user "your-username" password "your-password" noauth
```

Once connected, the remote device will have its own public IP address through the NTT line.

## Building

```
make
```

## Requirements

- Go 1.20 or higher
- Linux system with root access (for raw socket operations)
- Administrative privileges on network interfaces