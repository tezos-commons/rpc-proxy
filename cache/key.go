package cache

import (
	"encoding/binary"
	"sync"

	"github.com/cespare/xxhash/v2"
)

var keyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 512)
		return &b
	},
}

// HashKey hashes variadic byte-slice parts into a fixed 16-byte string key
// using xxhash (two passes with different seeds). Parts are separated by null
// bytes to prevent collisions.
func HashKey(parts ...[]byte) string {
	bp := keyBufPool.Get().(*[]byte)
	buf := (*bp)[:0]
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, 0)
		}
		buf = append(buf, p...)
	}

	h1 := xxhash.Sum64(buf)
	// Second hash with a length-prefix seed to get 128 bits of collision resistance
	var seed [8]byte
	binary.LittleEndian.PutUint64(seed[:], uint64(len(buf)))
	var d xxhash.Digest
	d.Write(seed[:])
	d.Write(buf)
	h2 := d.Sum64()

	var key [16]byte
	binary.LittleEndian.PutUint64(key[:8], h1)
	binary.LittleEndian.PutUint64(key[8:], h2)

	*bp = buf
	keyBufPool.Put(bp)
	return string(key[:])
}
