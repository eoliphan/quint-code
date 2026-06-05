package embedding

import (
	"encoding/binary"
	"math"
)

// EncodeVector serializes a float32 vector as little-endian bytes (4 bytes per
// element) for compact BLOB storage. The inverse of DecodeVector.
func EncodeVector(vector []float32) []byte {
	buffer := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(buffer[index*4:], math.Float32bits(value))
	}
	return buffer
}

// DecodeVector parses little-endian float32 bytes back into a vector. A buffer
// whose length is not a multiple of 4 yields nil (corrupt/foreign blob).
func DecodeVector(buffer []byte) []float32 {
	if len(buffer) == 0 || len(buffer)%4 != 0 {
		return nil
	}
	vector := make([]float32, len(buffer)/4)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(buffer[index*4:]))
	}
	return vector
}
