package clientassets

import (
	"encoding/binary"
	"math/bits"
)

const (
	nativeHashBlockBytes      = 64
	nativeHashDigestBytes     = 32
	nativeCipherBlockBytes    = 16
	nativeCipherKeyBytes      = 32
	nativeCipherScheduleBytes = 240
	nativeCipherRounds        = 14
)

// nativeSHA256 reproduces FUN_00111334's padding-length quirk: the encoded
// length includes padding before the final eight bytes. This is compatibility
// code for this client's assets, not a general-purpose cryptographic primitive.
func nativeSHA256(message []byte) [nativeHashDigestBytes]byte {
	state := [8]uint32{0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19}
	compress := func(block []byte) {
		var w [64]uint32
		for i := 0; i < 16; i++ {
			w[i] = binary.BigEndian.Uint32(block[i*4:])
		}
		for i := 16; i < len(w); i++ {
			a, b := w[i-15], w[i-2]
			w[i] = w[i-16] + (bits.RotateLeft32(a, -7) ^ bits.RotateLeft32(a, -18) ^ (a >> 3)) + w[i-7] + (bits.RotateLeft32(b, -17) ^ bits.RotateLeft32(b, -19) ^ (b >> 10))
		}
		a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
		for i := range w {
			t1 := h + (bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)) + ((e & f) ^ (^e & g)) + nativeHashRoundConstants[i] + w[i]
			t2 := (bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)) + ((a & b) ^ (a & c) ^ (b & c))
			h, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
		}
		state[0] += a
		state[1] += b
		state[2] += c
		state[3] += d
		state[4] += e
		state[5] += f
		state[6] += g
		state[7] += h
	}
	original := len(message)
	for len(message) >= nativeHashBlockBytes {
		compress(message[:nativeHashBlockBytes])
		message = message[nativeHashBlockBytes:]
	}
	padding := nativeHashBlockBytes - 8 - len(message)
	if padding <= 0 {
		padding += nativeHashBlockBytes
	}
	tail := make([]byte, len(message)+padding+8)
	copy(tail, message)
	tail[len(message)] = 0x80
	binary.BigEndian.PutUint64(tail[len(tail)-8:], uint64(original+padding)*8)
	for len(tail) > 0 {
		compress(tail[:nativeHashBlockBytes])
		tail = tail[nativeHashBlockBytes:]
	}
	var digest [nativeHashDigestBytes]byte
	for i, value := range state {
		binary.BigEndian.PutUint32(digest[i*4:], value)
	}
	return digest
}

func nativeHMAC(key, message []byte) [nativeHashDigestBytes]byte {
	if len(key) > nativeHashBlockBytes {
		digest := nativeSHA256(key)
		key = digest[:]
	}
	inner := make([]byte, nativeHashBlockBytes+len(message))
	outer := make([]byte, nativeHashBlockBytes+nativeHashDigestBytes)
	for i := 0; i < nativeHashBlockBytes; i++ {
		var value byte
		if i < len(key) {
			value = key[i]
		}
		inner[i] = value ^ 0x36
		outer[i] = value ^ 0x5c
	}
	copy(inner[nativeHashBlockBytes:], message)
	digest := nativeSHA256(inner)
	copy(outer[nativeHashBlockBytes:], digest[:])
	return nativeSHA256(outer)
}

// nativeCipher reproduces FUN_001116b8's key-schedule source indexing: both
// the output position and the byte index advance inside a word. The round
// operations otherwise follow AES. Standard crypto/aes cannot replace it.
type nativeCipher [nativeCipherScheduleBytes]byte

func newNativeCipher(key []byte) nativeCipher {
	var schedule nativeCipher
	copy(schedule[:], key)
	position := nativeCipherKeyBytes
	rcon := byte(1)
	for position < len(schedule) {
		var word [4]byte
		copy(word[:], schedule[position-4:position])
		if position%nativeCipherKeyBytes == 0 {
			word = [4]byte{nativeCipherSBox[word[1]], nativeCipherSBox[word[2]], nativeCipherSBox[word[3]], nativeCipherSBox[word[0]]}
			word[0] ^= rcon
			rcon = nativeXTime(rcon)
		} else if position%nativeCipherKeyBytes == nativeCipherBlockBytes {
			for i := range word {
				word[i] = nativeCipherSBox[word[i]]
			}
		}
		for i, value := range word {
			schedule[position] = schedule[position-nativeCipherKeyBytes+i] ^ value
			position++
		}
	}
	return schedule
}
func nativeXTime(value byte) byte {
	if value&0x80 != 0 {
		return value<<1 ^ 0x1b
	}
	return value << 1
}
func (schedule *nativeCipher) encrypt(block [nativeCipherBlockBytes]byte) [nativeCipherBlockBytes]byte {
	for i := range block {
		block[i] ^= schedule[i]
	}
	for round := 1; round <= nativeCipherRounds; round++ {
		for i := range block {
			block[i] = nativeCipherSBox[block[i]]
		}
		previous := block
		for row := 1; row < 4; row++ {
			for column := 0; column < 4; column++ {
				block[row+4*column] = previous[row+4*((column+row)%4)]
			}
		}
		if round < nativeCipherRounds {
			for at := 0; at < len(block); at += 4 {
				a, b, c, d := block[at], block[at+1], block[at+2], block[at+3]
				block[at] = nativeXTime(a) ^ nativeXTime(b) ^ b ^ c ^ d
				block[at+1] = a ^ nativeXTime(b) ^ nativeXTime(c) ^ c ^ d
				block[at+2] = a ^ b ^ nativeXTime(c) ^ nativeXTime(d) ^ d
				block[at+3] = nativeXTime(a) ^ a ^ b ^ c ^ nativeXTime(d)
			}
		}
		for i := range block {
			block[i] ^= schedule[round*nativeCipherBlockBytes+i]
		}
	}
	return block
}
func nativeCTR(key, nonce, data []byte) []byte {
	schedule := newNativeCipher(key)
	var counter [nativeCipherBlockBytes]byte
	copy(counter[:], nonce)
	output := make([]byte, len(data))
	for at := 0; at < len(data); at += nativeCipherBlockBytes {
		stream := schedule.encrypt(counter)
		for i := 0; i < nativeCipherBlockBytes && at+i < len(data); i++ {
			output[at+i] = data[at+i] ^ stream[i]
		}
		for i := len(counter) - 1; i >= 0; i-- {
			counter[i]++
			if counter[i] != 0 {
				break
			}
		}
	}
	return output
}

var nativeHashRoundConstants = [64]uint32{0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2}
var nativeCipherSBox = [256]byte{0x63, 0x7c, 0x77, 0x7b, 0xf2, 0x6b, 0x6f, 0xc5, 0x30, 0x01, 0x67, 0x2b, 0xfe, 0xd7, 0xab, 0x76, 0xca, 0x82, 0xc9, 0x7d, 0xfa, 0x59, 0x47, 0xf0, 0xad, 0xd4, 0xa2, 0xaf, 0x9c, 0xa4, 0x72, 0xc0, 0xb7, 0xfd, 0x93, 0x26, 0x36, 0x3f, 0xf7, 0xcc, 0x34, 0xa5, 0xe5, 0xf1, 0x71, 0xd8, 0x31, 0x15, 0x04, 0xc7, 0x23, 0xc3, 0x18, 0x96, 0x05, 0x9a, 0x07, 0x12, 0x80, 0xe2, 0xeb, 0x27, 0xb2, 0x75, 0x09, 0x83, 0x2c, 0x1a, 0x1b, 0x6e, 0x5a, 0xa0, 0x52, 0x3b, 0xd6, 0xb3, 0x29, 0xe3, 0x2f, 0x84, 0x53, 0xd1, 0x00, 0xed, 0x20, 0xfc, 0xb1, 0x5b, 0x6a, 0xcb, 0xbe, 0x39, 0x4a, 0x4c, 0x58, 0xcf, 0xd0, 0xef, 0xaa, 0xfb, 0x43, 0x4d, 0x33, 0x85, 0x45, 0xf9, 0x02, 0x7f, 0x50, 0x3c, 0x9f, 0xa8, 0x51, 0xa3, 0x40, 0x8f, 0x92, 0x9d, 0x38, 0xf5, 0xbc, 0xb6, 0xda, 0x21, 0x10, 0xff, 0xf3, 0xd2, 0xcd, 0x0c, 0x13, 0xec, 0x5f, 0x97, 0x44, 0x17, 0xc4, 0xa7, 0x7e, 0x3d, 0x64, 0x5d, 0x19, 0x73, 0x60, 0x81, 0x4f, 0xdc, 0x22, 0x2a, 0x90, 0x88, 0x46, 0xee, 0xb8, 0x14, 0xde, 0x5e, 0x0b, 0xdb, 0xe0, 0x32, 0x3a, 0x0a, 0x49, 0x06, 0x24, 0x5c, 0xc2, 0xd3, 0xac, 0x62, 0x91, 0x95, 0xe4, 0x79, 0xe7, 0xc8, 0x37, 0x6d, 0x8d, 0xd5, 0x4e, 0xa9, 0x6c, 0x56, 0xf4, 0xea, 0x65, 0x7a, 0xae, 0x08, 0xba, 0x78, 0x25, 0x2e, 0x1c, 0xa6, 0xb4, 0xc6, 0xe8, 0xdd, 0x74, 0x1f, 0x4b, 0xbd, 0x8b, 0x8a, 0x70, 0x3e, 0xb5, 0x66, 0x48, 0x03, 0xf6, 0x0e, 0x61, 0x35, 0x57, 0xb9, 0x86, 0xc1, 0x1d, 0x9e, 0xe1, 0xf8, 0x98, 0x11, 0x69, 0xd9, 0x8e, 0x94, 0x9b, 0x1e, 0x87, 0xe9, 0xce, 0x55, 0x28, 0xdf, 0x8c, 0xa1, 0x89, 0x0d, 0xbf, 0xe6, 0x42, 0x68, 0x41, 0x99, 0x2d, 0x0f, 0xb0, 0x54, 0xbb, 0x16}
