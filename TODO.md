# PPPoE Proxy TODO List

## Current Status

Basic project structure is in place with:
- Command-line flag parsing
- Raw socket setup for PPPoE discovery (0x8863) and session (0x8864) packets
- Host-Uniq tag rewriting logic for server-side authentication
- Packet capture logic

## Remaining Tasks

### High Priority

1. **Client-Server Communication**
   - [ ] Implement TCP server in server mode
   - [ ] Implement TCP client in client mode
   - [ ] Add protocol for tunnel establishment
   - [ ] Create packet serialization/deserialization

2. **Packet Forwarding**
   - [ ] Implement server-side packet forwarding to actual PPPoE server
   - [ ] Implement client-side packet forwarding to local interface
   - [ ] Add error handling for packet forwarding failures

3. **Authentication and Security**
   - [ ] Enhance Host-Uniq tag rewriting with stronger cryptographic methods
   - [ ] Add verification of client identity using shared secret
   - [ ] Implement connection handshake with challenge-response

### Medium Priority

4. **Reliability and Robustness**
   - [ ] Add reconnection logic for client mode
   - [ ] Implement session tracking and cleanup
   - [ ] Add timeout handling for inactive sessions
   - [ ] Create graceful shutdown with session termination

5. **Performance Optimization**
   - [ ] Add packet batching for network efficiency
   - [ ] Optimize memory usage for packet buffers
   - [ ] Add concurrency controls for high-volume traffic

### Nice to Have

6. **Monitoring and Logging**
   - [ ] Add detailed logging with levels (debug, info, warning, error)
   - [ ] Implement metrics collection (packets, bytes, sessions)
   - [ ] Create status API for monitoring

7. **Configuration and Management**
   - [ ] Add support for configuration file
   - [ ] Implement runtime configuration changes
   - [ ] Create basic admin interface

## Implementation Plan

### Phase 1: Core Functionality
- Complete TCP communication between client and server
- Implement basic packet forwarding
- Add simple authentication

### Phase 2: Robustness
- Add error handling and recovery
- Implement session tracking and timeout handling
- Enhance security features

### Phase 3: Performance and Polish
- Optimize for performance
- Add monitoring and logging
- Implement advanced configuration options