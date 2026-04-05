package cache

import (
	"bytes"
	"sync"

	"github.com/klauspost/compress/gzip"
)

var gzipWriterPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

var gzipBufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// Compress gzip-compresses data using pooled writers.
func Compress(data []byte) []byte {
	buf := gzipBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	w := gzipWriterPool.Get().(*gzip.Writer)
	w.Reset(buf)

	w.Write(data)
	w.Close()

	// Copy result BEFORE returning buffer to pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())

	gzipBufPool.Put(buf)
	gzipWriterPool.Put(w)

	return result
}

// gzipMinSize is the minimum body size worth compressing. Below this,
// gzip headers (~20 bytes) and CPU cost exceed bandwidth savings.
const gzipMinSize = 256

var gzipBytes = []byte("gzip")

// AcceptsGzip checks if the Accept-Encoding header contains gzip.
func AcceptsGzip(acceptEncoding []byte) bool {
	return bytes.Contains(acceptEncoding, gzipBytes)
}
