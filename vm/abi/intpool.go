package abi

import (
	"math/big"
	"sync"
)

// poolLimit caps the number of [big.Int]s a single [IntPool] may
// retain. Values returned via [IntPool.Put] beyond this limit are
// dropped so the pool does not grow unbounded.
const poolLimit = 256

// IntPool is a pool of big integers that can be reused for all
// big.Int operations. Used by the ABI unpacker to amortize
// allocations during bulk decoding.
type IntPool struct {
	pool *stack
}

// newIntPool returns a fresh, empty pool.
func newIntPool() *IntPool {
	return &IntPool{pool: newStack()}
}

// Get retrieves a big int from the pool, allocating one if the pool
// is empty. The returned int's amount is arbitrary and will not be
// zeroed — callers must initialize it themselves.
func (p *IntPool) Get() *big.Int {
	if p.pool.len() > 0 {
		return p.pool.pop()
	}
	return new(big.Int)
}

// GetZero retrieves a big int from the pool, setting it to zero or
// allocating a new one if the pool is empty.
func (p *IntPool) GetZero() *big.Int {
	if p.pool.len() > 0 {
		return p.pool.pop().SetUint64(0)
	}
	return new(big.Int)
}

// Put returns one or more allocated big ints to the pool. Values are
// saved as is; neither Put nor Get zeroes the ints out. Returns
// silently when the pool is at [poolLimit].
func (p *IntPool) Put(is ...*big.Int) {
	if len(p.pool.data) > poolLimit {
		return
	}
	for _, i := range is {
		p.pool.push(i)
	}
}

// intPoolPool manages a pool of [IntPool]s — one outer pool per
// codec call so concurrent unpack operations do not contend on a
// single inner pool.
type intPoolPool struct {
	pools []*IntPool
	lock  sync.Mutex
}

// poolDefaultCap is the initial capacity of [intPoolPool.pools].
const poolDefaultCap = 25

// PoolOfIntPools is the package-level pool of [IntPool]s. The
// unpacker's [lengthPrefixPointsTo] borrows a pool here, performs its
// arithmetic, and returns it.
var PoolOfIntPools = &intPoolPool{
	pools: make([]*IntPool, 0, poolDefaultCap),
}

// Get is looking for an available pool to return; allocates a fresh
// one if none is available.
func (ipp *intPoolPool) Get() *IntPool {
	ipp.lock.Lock()
	defer ipp.lock.Unlock()

	if len(PoolOfIntPools.pools) > 0 {
		ip := ipp.pools[len(ipp.pools)-1]
		ipp.pools = ipp.pools[:len(ipp.pools)-1]
		return ip
	}
	return newIntPool()
}

// Put returns ip to the pool that had been allocated with [Get].
// Drops ip silently when the outer pool is at capacity.
func (ipp *intPoolPool) Put(ip *IntPool) {
	ipp.lock.Lock()
	defer ipp.lock.Unlock()

	if len(ipp.pools) < cap(ipp.pools) {
		ipp.pools = append(ipp.pools, ip)
	}
}

// stack is the per-[IntPool] LIFO ring buffer of [big.Int] handles.
type stack struct {
	data []*big.Int
}

// newStack returns a fresh stack with initial capacity 1024.
func newStack() *stack {
	return &stack{data: make([]*big.Int, 0, 1024)}
}

// push appends d to the stack.
func (st *stack) push(d *big.Int) {
	st.data = append(st.data, d)
}

// pop removes and returns the top of the stack. Panics on empty
// stack — caller must check [stack.len] first.
func (st *stack) pop() (ret *big.Int) {
	ret = st.data[len(st.data)-1]
	st.data = st.data[:len(st.data)-1]
	return
}

// len returns the current depth.
func (st *stack) len() int {
	return len(st.data)
}
