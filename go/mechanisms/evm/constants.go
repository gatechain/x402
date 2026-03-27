package evm

import (
	"math/big"
)

const (
	// Scheme identifier
	SchemeExact = "exact"

	// Default token decimals for USDC
	DefaultDecimals = 6

	// EIP-3009 function names
	FunctionTransferWithAuthorization = "transferWithAuthorization"
	FunctionReceiveWithAuthorization  = "receiveWithAuthorization"
	FunctionAuthorizationState        = "authorizationState"

	// Transaction status
	TxStatusSuccess = 1
	TxStatusFailed  = 0

	// Default validity period (1 hour)
	DefaultValidityPeriod = 3600 // seconds

	// ERC-6492 magic value (last 32 bytes of wrapped signature)
	// This is bytes32(uint256(keccak256("erc6492.invalid.signature")) - 1)
	ERC6492MagicValue = "0x6492649264926492649264926492649264926492649264926492649264926492"

	// EIP-1271 magic value (returned by isValidSignature on success)
	EIP1271MagicValue = "0x1626ba7e"

	// Error codes matching TypeScript implementation
	ErrInvalidSignature            = "invalid_exact_evm_payload_signature"
	ErrUndeployedSmartWallet       = "invalid_exact_evm_payload_undeployed_smart_wallet"
	ErrSmartWalletDeploymentFailed = "smart_wallet_deployment_failed"
)

var (
	// Network chain IDs
	ChainIDGateLayerTestnet = big.NewInt(10087) // Gate Layer Testnet chain ID (0x2767)
	ChainIDEthereumMainnet  = big.NewInt(1)     // Ethereum mainnet chain ID
	ChainIDBaseMainnet      = big.NewInt(8453)  // Base mainnet chain ID
	ChainIDBSCMainnet       = big.NewInt(56)    // BNB Smart Chain mainnet
	ChainIDBSCTestnet       = big.NewInt(97)    // BNB Smart Chain testnet (Chapel)

	// Network configurations
	// See DEFAULT_ASSET.md for guidelines on adding new chains
	//
	// Default Asset Selection Policy:
	// - Each chain has the right to determine its own default stablecoin
	// - If the chain has officially endorsed a stablecoin, that asset should be used
	// - If no official stance exists, the chain team should make the selection
	//
	// NOTE:
	// - EIP-3009 stablecoins are supported as the default EVM exact flow.
	// - Permit2-based exact flow is also supported in dedicated Permit2 integrations/examples.
	// - Generic ERC-20 coverage across all networks/assets is still evolving.
	NetworkConfigs = map[string]NetworkConfig{
		// Gate Layer Testnet
		"gatelayer_testnet": {
			ChainID: ChainIDGateLayerTestnet,
			DefaultAsset: AssetInfo{
				Address:  "0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF", // USDC on Gate Layer Testnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// Gate Layer Testnet (CAIP-2 format)
		"eip155:10087": {
			ChainID: ChainIDGateLayerTestnet,
			DefaultAsset: AssetInfo{
				Address:  "0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF", // USDC on Gate Layer Testnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// Ethereum mainnet (CAIP-2 format)
		"eip155:1": {
			ChainID: ChainIDEthereumMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC on Ethereum mainnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// Ethereum mainnet (short name)
		"eth": {
			ChainID: ChainIDEthereumMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC on Ethereum mainnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// Base mainnet (CAIP-2 format)
		"eip155:8453": {
			ChainID: ChainIDBaseMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // USDC on Base mainnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// Base mainnet (short name)
		"base": {
			ChainID: ChainIDBaseMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // USDC on Base mainnet
				Name:     "USDC",
				Version:  "2",
				Decimals: DefaultDecimals,
			},
		},
		// BSC mainnet — Binance-Peg USDT (18 decimals)
		"eip155:56": {
			ChainID: ChainIDBSCMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0x55d398326f99059fF775485246999027B3197955",
				Name:     "USDT",
				Version:  "1",
				Decimals: 18,
			},
		},
		"bsc": {
			ChainID: ChainIDBSCMainnet,
			DefaultAsset: AssetInfo{
				Address:  "0x55d398326f99059fF775485246999027B3197955",
				Name:     "USDT",
				Version:  "1",
				Decimals: 18,
			},
		},
		// BSC testnet (Chapel) — common test USDT; override with env in demos if needed
		"eip155:97": {
			ChainID: ChainIDBSCTestnet,
			DefaultAsset: AssetInfo{
				Address:  "0x337610d27c682E347C9cD60BD4b3b107C9d34dDd",
				Name:     "USDT",
				Version:  "1",
				Decimals: 6,
			},
		},
		"bsc-testnet": {
			ChainID: ChainIDBSCTestnet,
			DefaultAsset: AssetInfo{
				Address:  "0x337610d27c682E347C9cD60BD4b3b107C9d34dDd",
				Name:     "USDT",
				Version:  "1",
				Decimals: 6,
			},
		},
	}

	// GateLayerTestnetDomainSeparators maps token address (lowercase) to DOMAIN_SEPARATOR from chain (hex with 0x).
	// Used for EIP-3009 signing when the contract's domain separator must match exactly.
	GateLayerTestnetDomainSeparators = map[string]string{
		"0x9be8df37c788b244cfc28e46654ad5ec28a880af": "0x2c2d6b621e73a4a094449d1894717413742130fb20149ec48340ca0354d1a707", // USDC
		"0x081ff58e7d7105ad400f4cc76becfd8684013a4d": "0x7c6ddc1021fbf24f4dbe62b331d83549a44e91bee3d396a33171bebe573b0fab", // additional token
	}

	// EIP-3009 ABI for transferWithAuthorization with v,r,s (EOA signatures)
	TransferWithAuthorizationVRSABI = []byte(`[
		{
			"inputs": [
				{"name": "from", "type": "address"},
				{"name": "to", "type": "address"},
				{"name": "value", "type": "uint256"},
				{"name": "validAfter", "type": "uint256"},
				{"name": "validBefore", "type": "uint256"},
				{"name": "nonce", "type": "bytes32"},
				{"name": "v", "type": "uint8"},
				{"name": "r", "type": "bytes32"},
				{"name": "s", "type": "bytes32"}
			],
			"name": "transferWithAuthorization",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`)

	// EIP-3009 ABI for transferWithAuthorization with bytes signature (smart wallets)
	TransferWithAuthorizationBytesABI = []byte(`[
		{
			"inputs": [
				{"name": "from", "type": "address"},
				{"name": "to", "type": "address"},
				{"name": "value", "type": "uint256"},
				{"name": "validAfter", "type": "uint256"},
				{"name": "validBefore", "type": "uint256"},
				{"name": "nonce", "type": "bytes32"},
				{"name": "signature", "type": "bytes"}
			],
			"name": "transferWithAuthorization",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`)

	// Legacy: Combined ABI (deprecated, use specific ABIs above)
	TransferWithAuthorizationABI = TransferWithAuthorizationVRSABI

	// ABI for authorizationState check
	AuthorizationStateABI = []byte(`[
		{
			"inputs": [
				{"name": "authorizer", "type": "address"},
				{"name": "nonce", "type": "bytes32"}
			],
			"name": "authorizationState",
			"outputs": [{"name": "", "type": "bool"}],
			"stateMutability": "view",
			"type": "function"
		}
	]`)
)
