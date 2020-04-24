// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package blobloom implements blocked Bloom filters.
//
// Blocked Bloom filters are an approximate set data structure: if a key has
// been added to a filter, a lookup of that key returns true, but if the key
// has not been added, there is a non-zero probability that the lookup still
// returns true (a false positive). It follows that, if the lookup for a key
// returns false, that key has not been added to the filter.
//
// In this package, keys are represented exclusively as hashes. Client code
// is responsible for supplying two 32-bit hash values for a key. No hash
// function is provided, since the "right" hash function for an application
// depends on the data the application processes.
//
// Compared to standard Bloom filters, blocked Bloom filters use the CPU
// cache more efficiently. A blocked Bloom filter is an array of ordinary
// Bloom filters of fixed size BlockBits (the blocks). The first hash of a
// key selects the block to use.
//
// To achieve the same false positive rate (FPR) as a standard Bloom filter,
// a blocked Bloom filter requires more memory. For an FPR of at most 2e-6
// (two in a million), it uses ~20% more memory. At 1e-10, the space required
// is double that of standard Bloom filter.
//
// For more details, see the 2010 paper by Putze, Sanders and Singler,
// https://algo2.iti.kit.edu/documents/cacheefficientbloomfilters-jea.pdf.
package blobloom

import (
	"math"
	"math/bits"
	"sync/atomic"
)

// BlockBits is the number of bits per block and the minimum number of bits
// in a Filter.
//
// The value of this constant is chosen to match the L1 cache line size
// of popular architectures (386, amd64, arm64).
const BlockBits = 512

// MaxBits is the maximum number of bits supported by a Filter.
const MaxBits = BlockBits << 32 // 256GiB.

// A Filter is a blocked Bloom filter.
type Filter struct {
	b []block // Shards.
	k int     // Number of hash functions required.
}

// New constructs a Bloom filter with given numbers of bits and hash functions.
//
// The number of bits should be at least BlockBits; smaller values are silently
// increased.
//
// The number of hash functions uses is silently increased to two.
// The client passes the first two hashes for every key to Add and Has,
// which synthesize all following hashes from the two values passed in.
func New(nbits uint64, nhashes int) *Filter {
	if nbits < 1 {
		nbits = BlockBits
	}
	if nhashes < 2 {
		nhashes = 2
	}
	if nbits > MaxBits {
		panic("nbits exceeds MaxBits")
	}

	// Round nbits up to a multiple of BlockBits.
	if nbits%BlockBits != 0 {
		nbits += BlockBits - nbits%BlockBits
	}

	return &Filter{
		b: make([]block, nbits/BlockBits),
		k: nhashes,
	}
}

// Add insert a key with hash value h into f.
//
// The upper and lower half of h are treated as two independent hashes.
// These are used to derive further values using the enhanced double hashing
// construction of Dillinger and Manolios,
// https://www.ccs.neu.edu/home/pete/pub/bloom-filters-verification.pdf.
func (f *Filter) Add(h uint64) {
	f.Add2(uint32(h>>32), uint32(h))
}

// Add2 inserts a key with hash values h1 and h2 into f.
//
// Add2 is equivalent to Add(h1<<32 | h2).
func (f *Filter) Add2(h1, h2 uint32) {
	i := reducerange(h1, uint32(len(f.b)))
	b := &f.b[i]

	for i := 0; i+1 < f.k; i++ {
		h1, h2 = doublehash(h1, h2, i)
		b.setbit(h1)
	}
}

// AddAtomic atomically inserts a key with hash value h into f.
//
// This is a synchronized version of Add.
// Multiple goroutines may call AddAtomic and AddAtomic2 concurrently,
// though no goroutines should call any other methods on f concurrently
// with these methods.
func (f *Filter) AddAtomic(h uint64) {
	f.AddAtomic2(uint32(h>>32), uint32(h))
}

// AddAtomic2 atomically inserts a key with hash values h1 and h2 into f.
//
// AddAtomic2 is equivalent to AddAtomic(h1<<32 | h2).
func (f *Filter) AddAtomic2(h1, h2 uint32) {
	i := reducerange(h1, uint32(len(f.b)))
	b := &f.b[i]

	for i := 0; i+1 < f.k; i++ {
		h1, h2 = doublehash(h1, h2, i)
		b.setbitAtomic(h1)
	}
}

// Cardinality estimates the number of distinct keys added to f.
//
// The estimate is the maximum likelihood estimate of Papapetrou, Siberski
// and Nejdl (https://www.win.tue.nl/~opapapetrou/papers/Bloomfilters-DAPD.pdf)
// summed over the blocks.
func (f *Filter) Cardinality() float64 {
	log1p := math.Log1p
	k := float64(f.k) - 1

	var n float64
	for _, b := range f.b {
		ones := float64(b.onescount())
		n += log1p(-ones/BlockBits) / (k * log1p(-1./BlockBits))
	}
	return n
}

// Clear resets f to its empty state.
func (f *Filter) Clear() {
	for i := range f.b {
		f.b[i] = block{}
	}
}

// Has reports whether a key with hash value h has been added.
// It may return a false positive.
func (f *Filter) Has(h uint64) bool {
	return f.Has2(uint32(h>>32), uint32(h))
}

// Has2 reports whether a key with hash values h1 and h2 has been added.
// It may return a false positive.
//
// Has2 is equivalent to Has(h1<<32 | h2).
func (f *Filter) Has2(h1, h2 uint32) bool {
	i := reducerange(h1, uint32(len(f.b)))
	b := &f.b[i]

	for i := 0; i+1 < f.k; i++ {
		h1, h2 = doublehash(h1, h2, i)
		if !b.getbit(h1) {
			return false
		}
	}
	return true
}

// doublehash generates the hash values n1, n2 to use in iteration i of
// enhanced double hashing from the values h1, h2 of the previous iteration.
func doublehash(h1, h2 uint32, i int) (uint32, uint32) {
	h1 = h1 + h2
	h2 = h2 + uint32(i)
	return h1, h2
}

// reducerange maps i to an integer in the range [0,n).
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
func reducerange(i, n uint32) uint32 {
	return uint32((uint64(i) * uint64(n)) >> 32)
}

// NumBits returns the number of bits of f.
func (f *Filter) NumBits() uint64 {
	return BlockBits * uint64(len(f.b))
}

// Union sets f to the union of f and g.
//
// Union panics when f and g do not have the same number of bits and
// hash functions. Both Filters must be using the same hash function(s),
// but Union cannot check this.
func (f *Filter) Union(g *Filter) {
	if len(f.b) != len(g.b) {
		panic("Bloom filters do not have the same number of bits")
	}
	if f.k != g.k {
		panic("Bloom filters do not have the same number of hash functions")
	}
	for i := range f.b {
		f.b[i].union(&g.b[i])
	}
}

const (
	wordSize  = 32
	blockSize = BlockBits / wordSize
)

// A block is a fixed-size Bloom filter, used as a shard of a Filter.
type block [blockSize]uint32

// getbit reports whether bit (i modulo BlockBits) is set.
func (b *block) getbit(i uint32) bool {
	const n = uint32(len(*b))
	x := (*b)[(i/wordSize)%n] & (1 << (i % wordSize))
	return x != 0
}

// setbit sets bit (i modulo BlockBits) of b.
func (b *block) setbit(i uint32) {
	const n = uint32(len(*b))
	(*b)[(i/wordSize)%n] |= 1 << (i % wordSize)
}

// setbit sets bit (i modulo BlockBits) of b, atomically.
func (b *block) setbitAtomic(i uint32) {
	const n = uint32(len(*b))
	bit := uint32(1) << (i % wordSize)
	p := &(*b)[(i/wordSize)%n]
	for {
		old := atomic.LoadUint32(p)
		if old&bit != 0 {
			// Checking here instead of checking the return value from
			// the CAS is between 25% and 50% faster on the benchmark.
			return
		}
		new := old | bit
		atomic.CompareAndSwapUint32(p, old, new)
	}
}

func (b *block) onescount() (n int) {
	n += bits.OnesCount32(b[0])
	n += bits.OnesCount32(b[1])
	n += bits.OnesCount32(b[2])
	n += bits.OnesCount32(b[3])
	n += bits.OnesCount32(b[4])
	n += bits.OnesCount32(b[5])
	n += bits.OnesCount32(b[6])
	n += bits.OnesCount32(b[7])
	n += bits.OnesCount32(b[8])
	n += bits.OnesCount32(b[9])
	n += bits.OnesCount32(b[10])
	n += bits.OnesCount32(b[11])
	n += bits.OnesCount32(b[12])
	n += bits.OnesCount32(b[13])
	n += bits.OnesCount32(b[14])
	n += bits.OnesCount32(b[15])
	return n
}

func (b *block) union(c *block) {
	b[0] |= c[0]
	b[1] |= c[1]
	b[2] |= c[2]
	b[3] |= c[3]
	b[4] |= c[4]
	b[5] |= c[5]
	b[6] |= c[6]
	b[7] |= c[7]
	b[8] |= c[8]
	b[9] |= c[9]
	b[10] |= c[10]
	b[11] |= c[11]
	b[12] |= c[12]
	b[13] |= c[13]
	b[14] |= c[14]
	b[15] |= c[15]
}
