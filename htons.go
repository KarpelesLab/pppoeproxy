package main

import (
	"encoding/binary"
)

// isLittleEndian checks whether the machine is little endian
func isLittleEndian() bool {
	// Create a byte slice with known values
	buf := []byte{0x12, 0x34}

	// If machine is little endian, the value will be interpreted as 0x3412
	// If machine is big endian, the value will be interpreted as 0x1234
	return binary.LittleEndian.Uint16(buf) == uint16(0x3412)
}

// littleEndianMachine is a global variable indicating whether the machine is little endian
var littleEndianMachine = isLittleEndian()

// htons converts a short (uint16) from host byte order to network byte order (big endian)
func htons(i uint16) uint16 {
	// Only swap if the machine is little endian
	if littleEndianMachine {
		// Simple byte swap using bit shifting
		return (i << 8) | (i >> 8)
	}
	return i
}

// ntohs converts a short (uint16) from network byte order (big endian) to host byte order
func ntohs(i uint16) uint16 {
	// Network to host is the same operation as host to network
	return htons(i)
}
