module github.com/greatroar/blobloom/benchmarks

go 1.14

require (
	github.com/bits-and-blooms/bloom/v3 v3.0.1
	github.com/cespare/xxhash/v2 v2.1.1
	github.com/d4l3k/messagediff v1.2.1 // indirect
	github.com/devopsfaith/bloomfilter v1.4.0
	github.com/greatroar/blobloom v0.6.0
	github.com/ipfs/bbloom v0.0.4
	github.com/tylertreat/BoomFilters v0.0.0-20210315201527-1a82519a3e43
	github.com/zeebo/xxh3 v0.12.0
	golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e // indirect
)

replace github.com/greatroar/blobloom => ../
