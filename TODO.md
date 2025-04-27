# PPPoE Proxy TODO List

## Current Status

Implementation includes:
- Command-line flag parsing
- Raw socket setup for PPPoE discovery (0x8863) and session (0x8864) packets
- Host-Uniq tag rewriting logic for server-side authentication
- Packet capture logic
- TCP client-server communication
- Protocol for packet transmission
- Packet forwarding between interfaces

## Remaining Tasks

### High Priority

1. **Client-Server Communication**
   - [x] Implement TCP server in server mode
   - [x] Implement TCP client in client mode
   - [x] Add protocol for tunnel establishment
   - [x] Create packet serialization/deserialization

2. **Packet Forwarding**
   - [x] Implement server-side packet forwarding to actual PPPoE server
   - [x] Implement client-side packet forwarding to local interface
   - [x] Packet loss handling (drop silently)

3. **Authentication and Security**
   - [x] Add IP-based client access control

### Medium Priority

4. **Reliability and Robustness**
   - [x] Add reconnection logic for client mode
   - [x] Implement ping/pong keepalive mechanism
   - [ ] Implement session tracking and cleanup
   - [x] Add timeout handling for inactive connections
   - [x] Create graceful shutdown with session termination

5. **Performance Optimization**
   - [ ] Add packet batching for network efficiency
   - [ ] Optimize memory usage for packet buffers
   - [ ] Add concurrency controls for high-volume traffic

### Nice to Have

6. **Monitoring and Logging**
   - [ ] Add detailed logging with levels (debug, info, warning, error)
   - [ ] Implement metrics collection (packets, bytes, sessions)
   - [ ] Create status API for monitoring

## Implementation Plan

### Completed
- Core functionality (TCP tunneling, packet forwarding, authentication)
- Robustness (reconnection, keepalive, graceful shutdown)
- Endian-aware implementation for cross-platform support

### Future Improvements
- Performance optimization for high-traffic scenarios
- Enhanced monitoring and logging capabilities
- Metrics collection for operational insights