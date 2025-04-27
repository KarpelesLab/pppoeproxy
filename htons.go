package main

import (
	"encoding/binary"
	"unsafe"
)

// isLittleEndian checks whether the machine is little endian
func isLittleEndian() bool {
	var i int32 = 0x01020304
	// Convert to byte slice to check the first byte
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	return *pb == 0x04
}

// littleEndianMachine is a global variable indicating whether the machine is little endian
var littleEndianMachine = isLittleEndian()

// htons converts a short (uint16) from host byte order to network byte order (big endian)
func htons(i uint16) uint16 {
	// Only swap if the machine is little endian
	if littleEndianMachine {
		return binary.BigEndian.Uint16(binary.LittleEndian.AppendUint16(nil, i))
	}
	return i
}

// ntohs converts a short (uint16) from network byte order (big endian) to host byte order
func ntohs(i uint16) uint16 {
	// Only swap if the machine is little endian
	if littleEndianMachine {
		return binary.LittleEndian.Uint16(binary.BigEndian.AppendUint16(nil, i))
	}
	return i
}
