package cache

import (
	"bytes"
	"io"
	"testing"

	"github.com/klauspost/compress/gzip"
)

func TestCompress_Roundtrip(t *testing.T) {
	original := []byte("hello world, this is a test of gzip compression")
	compressed := Compress(original)

	if len(compressed) == 0 {
		t.Fatal("compressed output is empty")
	}

	// Decompress
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	r.Close()

	if !bytes.Equal(original, decompressed) {
		t.Fatalf("roundtrip mismatch: got %q; want %q", decompressed, original)
	}
}

func TestCompress_EmptyInput(t *testing.T) {
	compressed := Compress(nil)
	if len(compressed) == 0 {
		t.Fatal("compressed output for nil should still produce valid gzip")
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	r.Close()

	if len(data) != 0 {
		t.Fatalf("expected empty decompressed data, got %q", data)
	}
}

func TestCompress_PoolReuse(t *testing.T) {
	// Exercise pool reuse to verify correctness after multiple compress calls
	for i := 0; i < 50; i++ {
		input := bytes.Repeat([]byte("data"), i+1)
		compressed := Compress(input)

		r, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			t.Fatalf("iteration %d: gzip.NewReader: %v", i, err)
		}
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("iteration %d: ReadAll: %v", i, err)
		}
		r.Close()

		if !bytes.Equal(input, got) {
			t.Fatalf("iteration %d: roundtrip mismatch", i)
		}
	}
}

func TestCompress_LargePayload(t *testing.T) {
	original := bytes.Repeat([]byte("abcdefghijklmnopqrstuvwxyz"), 10000)
	compressed := Compress(original)

	// Should actually compress well
	if len(compressed) >= len(original) {
		t.Fatalf("expected compression; compressed=%d, original=%d", len(compressed), len(original))
	}

	r, _ := gzip.NewReader(bytes.NewReader(compressed))
	got, _ := io.ReadAll(r)
	r.Close()

	if !bytes.Equal(original, got) {
		t.Fatal("roundtrip mismatch for large payload")
	}
}

func TestAcceptsGzip(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"deflate, gzip", true},
		{"br, gzip, zstd", true},
		{"deflate, br", false},
		{"", false},
		{"identity", false},
		{"GZIP", false}, // case-sensitive
	}

	for _, tt := range tests {
		got := AcceptsGzip([]byte(tt.input))
		if got != tt.expected {
			t.Errorf("AcceptsGzip(%q) = %v; want %v", tt.input, got, tt.expected)
		}
	}
}
