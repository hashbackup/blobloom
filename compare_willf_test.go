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

// To run the Blobloom benchmarks on willf/bloom, remove the "build ignore"
// line below, then
//
//     go test -tags "benchcompare willf" -bench=.
//
// The ignore constraint is there to prevent willf/bloom from ending up in
// go.mod and becoming a transitive dependency for all users.

// +build benchcompare willf
// +build ignore

package blobloom_test

import "github.com/willf/bloom"

type bloomFilter bloom.BloomFilter

func (f *bloomFilter) Add(hash []byte) {
	((*bloom.BloomFilter)(f)).Add(hash)
}

func (f *bloomFilter) Has(hash []byte) bool {
	return ((*bloom.BloomFilter)(f)).Test(hash)
}

func newBF(capacity int, fpr float64) *bloomFilter {
	f := bloom.NewWithEstimates(uint(capacity), fpr)
	return (*bloomFilter)(f)
}