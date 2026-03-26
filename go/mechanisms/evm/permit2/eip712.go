package permit2

import (
	"github.com/gatechain/x402/go/mechanisms/evm"
)

// PermitSingleTypes returns EIP-712 type definitions matching Uniswap Permit2 PermitSingle.
// Ref: https://github.com/Uniswap/permit2/blob/main/src/libraries/PermitHash.sol
func PermitSingleTypes() map[string][]evm.TypedDataField {
	return map[string][]evm.TypedDataField{
		"PermitDetails": {
			{Name: "token", Type: "address"},
			{Name: "amount", Type: "uint256"},
			{Name: "expiration", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
		},
		"PermitSingle": {
			{Name: "details", Type: "PermitDetails"},
			{Name: "spender", Type: "address"},
			{Name: "sigDeadline", Type: "uint256"},
		},
	}
}
