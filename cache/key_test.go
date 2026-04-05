package cache

import (
	"testing"
)

func TestHashKey_Consistent(t *testing.T) {
	a := HashKey([]byte("hello"), []byte("world"))
	b := HashKey([]byte("hello"), []byte("world"))
	if a != b {
		t.Fatal("HashKey not consistent for same input")
	}
}

func TestHashKey_FixedLength(t *testing.T) {
	k := HashKey([]byte("test"))
	if len(k) != 16 {
		t.Fatalf("HashKey length = %d; want 16", len(k))
	}

	k2 := HashKey([]byte("a"), []byte("very"), []byte("long"), []byte("input"))
	if len(k2) != 16 {
		t.Fatalf("HashKey length = %d; want 16", len(k2))
	}
}

func TestHashKey_DifferentInputsDifferentKeys(t *testing.T) {
	cases := []struct {
		name string
		a, b [][]byte
	}{
		{
			name: "different single parts",
			a:    [][]byte{[]byte("foo")},
			b:    [][]byte{[]byte("bar")},
		},
		{
			name: "different multi parts",
			a:    [][]byte{[]byte("hello"), []byte("world")},
			b:    [][]byte{[]byte("hello"), []byte("earth")},
		},
		{
			name: "different number of parts",
			a:    [][]byte{[]byte("abc")},
			b:    [][]byte{[]byte("ab"), []byte("c")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ka := HashKey(tc.a...)
			kb := HashKey(tc.b...)
			if ka == kb {
				t.Fatalf("expected different keys for different inputs")
			}
		})
	}
}

func TestHashKey_NullByteSeparationPreventsCollisions(t *testing.T) {
	// ["ab", "c"] vs ["a", "bc"] should produce different keys
	// because null-byte separator makes them: "ab\x00c" vs "a\x00bc"
	k1 := HashKey([]byte("ab"), []byte("c"))
	k2 := HashKey([]byte("a"), []byte("bc"))
	if k1 == k2 {
		t.Fatal("HashKey collision: [ab,c] vs [a,bc] should differ due to null-byte separator")
	}
}

func TestHashKey_EmptyParts(t *testing.T) {
	// Empty parts with separator should still be distinct
	k1 := HashKey([]byte(""), []byte("a"))
	k2 := HashKey([]byte("a"), []byte(""))
	if k1 == k2 {
		t.Fatal("HashKey collision: ['',a] vs [a,''] should differ")
	}

	// Single empty part vs no parts equivalent
	k3 := HashKey([]byte(""))
	k4 := HashKey([]byte("x"))
	if k3 == k4 {
		t.Fatal("empty and non-empty should differ")
	}
}

func BenchmarkHashKey(b *testing.B) {
	parts := [][]byte{[]byte("mainnet"), []byte("eth_blockNumber"), []byte("[]")}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		HashKey(parts...)
	}
}
