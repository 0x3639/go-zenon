package types

// The implemented sporks. Each entry corresponds to an upstream protocol
// upgrade gated through the spork contract. Activating a spork through
// governance lets the VM and verifier dispatch to the new code path; the IDs
// here must match the on-chain spork records exactly.
var (
	// AcceleratorSpork enables the accelerator project-funding contract.
	AcceleratorSpork = NewImplementedSpork("6d2b1e6cb4025f2f45533f0fe22e9b7ce2014d91cc960471045fa64eee5a6ba3")
	// HtlcSpork enables the hashed-timelock-contract embedded contract.
	HtlcSpork = NewImplementedSpork("ceb7e3808ef17ea910adda2f3ab547be4cdfb54de8400ce3683258d06be1354b")
	// BridgeAndLiquiditySpork enables the cross-chain bridge and the
	// liquidity-program reward distribution.
	BridgeAndLiquiditySpork = NewImplementedSpork("ddd43466769461c5b5d109c639da0f50a7eeb96ad6e7274b1928a35c431d7b1b")

	// ImplementedSporksMap is a lookup of every spork ID this binary knows
	// how to honor. The spork contract uses it to reject creation of unknown
	// sporks during boot.
	ImplementedSporksMap = map[Hash]bool{
		AcceleratorSpork.SporkId:        true,
		HtlcSpork.SporkId:               true,
		BridgeAndLiquiditySpork.SporkId: true,
	}
)

// ImplementedSpork is the binary-side handle for a spork: the canonical hash
// the on-chain spork record must match. Activation state itself lives in the
// spork contract's storage; this struct only carries the identity.
type ImplementedSpork struct {
	SporkId Hash
}

// NewImplementedSpork constructs an [ImplementedSpork] from a hex-encoded
// spork id. Panics on malformed input — the IDs are compile-time constants.
func NewImplementedSpork(SporkIdStr string) *ImplementedSpork {
	return &ImplementedSpork{
		SporkId: HexToHashPanic(SporkIdStr),
	}
}
