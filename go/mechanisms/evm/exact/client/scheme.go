package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gatechain/x402/go/mechanisms/evm"
	"github.com/gatechain/x402/go/types"
)

// ExactEvmScheme implements the SchemeNetworkClient interface for EVM exact payments (V2)
type ExactEvmScheme struct {
	signer    evm.ClientEvmSigner
	rpcURL    string            // Optional RPC URL for querying chain data
	ethClient *ethclient.Client // Optional ethclient for querying chain data
}

// NewExactEvmScheme creates a new ExactEvmScheme
func NewExactEvmScheme(signer evm.ClientEvmSigner) *ExactEvmScheme {
	return &ExactEvmScheme{
		signer: signer,
	}
}

// SetRPCURL sets the RPC URL for querying chain data (optional)
func (c *ExactEvmScheme) SetRPCURL(rpcURL string) error {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return fmt.Errorf("failed to connect to RPC: %w", err)
	}
	c.rpcURL = rpcURL
	c.ethClient = client
	return nil
}

// Scheme returns the scheme identifier
func (c *ExactEvmScheme) Scheme() string {
	return evm.SchemeExact
}

// CreatePaymentPayload creates a V2 payment payload for the exact scheme
func (c *ExactEvmScheme) CreatePaymentPayload(
	ctx context.Context,
	requirements types.PaymentRequirements,
) (types.PaymentPayload, error) {
	networkStr := string(requirements.Network)

	// Get chain ID - works for any EIP-155 network (eip155:CHAIN_ID)
	chainID, err := evm.GetEvmChainId(networkStr)
	if err != nil {
		return types.PaymentPayload{}, err
	}

	// Get asset info - works for any explicit address, or uses default if configured
	assetInfo, err := evm.GetAssetInfo(networkStr, requirements.Asset)
	if err != nil {
		return types.PaymentPayload{}, err
	}

	// Requirements.Amount is already in the smallest unit
	value, ok := new(big.Int).SetString(requirements.Amount, 10)
	if !ok {
		return types.PaymentPayload{}, fmt.Errorf(ErrInvalidAmount+": %s", requirements.Amount)
	}

	// Create nonce
	nonce, err := evm.CreateNonce()
	if err != nil {
		return types.PaymentPayload{}, err
	}

	// V2 specific: No buffer on validAfter (can use immediately)
	validAfter, validBefore := evm.CreateValidityWindow(time.Hour)

	// Extract extra fields for EIP-3009
	tokenName := assetInfo.Name
	tokenVersion := assetInfo.Version
	if requirements.Extra != nil {
		if name, ok := requirements.Extra["name"].(string); ok {
			tokenName = name
		}
		if ver, ok := requirements.Extra["version"].(string); ok {
			tokenVersion = ver
		}
	}

	// Create authorization
	authorization := evm.ExactEIP3009Authorization{
		From:        c.signer.Address(),
		To:          requirements.PayTo,
		Value:       value.String(),
		ValidAfter:  validAfter.String(),
		ValidBefore: validBefore.String(),
		Nonce:       nonce,
	}

	// For gatelayer_testnet with specific token, use hardcoded DOMAIN_SEPARATOR from chain
	if networkStr == "gatelayer_testnet" && assetInfo.Address == "0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF" {
		// Use hardcoded DOMAIN_SEPARATOR from chain: 0x2c2d6b621e73a4a094449d1894717413742130fb20149ec48340ca0354d1a707
		domainSeparator, _ := hex.DecodeString("2c2d6b621e73a4a094449d1894717413742130fb20149ec48340ca0354d1a707")
		if len(domainSeparator) == 32 {
			signature, err := c.signWithDomainSeparator(ctx, authorization, domainSeparator)
			if err == nil {
				evmPayload := &evm.ExactEIP3009Payload{
					Signature:     evm.BytesToHex(signature),
					Authorization: authorization,
				}
				return types.PaymentPayload{
					X402Version: 2,
					Payload:     evmPayload.ToMap(),
				}, nil
			}
		}
	}

	// Sign the authorization (fallback to standard method)
	signature, err := c.signAuthorization(ctx, authorization, chainID, assetInfo.Address, tokenName, tokenVersion)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf(ErrFailedToSignAuthorization+": %w", err)
	}

	// Create EVM payload
	evmPayload := &evm.ExactEIP3009Payload{
		Signature:     evm.BytesToHex(signature),
		Authorization: authorization,
	}

	// Return partial V2 payload (core will add accepted, resource, extensions)
	return types.PaymentPayload{
		X402Version: 2,
		Payload:     evmPayload.ToMap(),
	}, nil
}

// signAuthorization signs the EIP-3009 authorization using EIP-712
func (c *ExactEvmScheme) signAuthorization(
	ctx context.Context,
	authorization evm.ExactEIP3009Authorization,
	chainID *big.Int,
	verifyingContract string,
	tokenName string,
	tokenVersion string,
) ([]byte, error) {
	// Try to query DOMAIN_SEPARATOR from chain if RPC is configured
	var domainSeparator []byte
	if c.ethClient != nil {
		domainSep, err := c.queryDomainSeparator(ctx, verifyingContract)
		if err == nil {
			domainSeparator = domainSep
		}
	}

	// If we have domain separator from chain, use it directly
	if domainSeparator != nil {
		return c.signWithDomainSeparator(ctx, authorization, domainSeparator)
	}

	// Fallback to standard EIP-712 signing with name/version
	domain := evm.TypedDataDomain{
		Name:              tokenName,
		Version:           tokenVersion,
		ChainID:           chainID,
		VerifyingContract: verifyingContract,
	}

	types := map[string][]evm.TypedDataField{
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"TransferWithAuthorization": {
			{Name: "from", Type: "address"},
			{Name: "to", Type: "address"},
			{Name: "value", Type: "uint256"},
			{Name: "validAfter", Type: "uint256"},
			{Name: "validBefore", Type: "uint256"},
			{Name: "nonce", Type: "bytes32"},
		},
	}

	value, _ := new(big.Int).SetString(authorization.Value, 10)
	validAfter, _ := new(big.Int).SetString(authorization.ValidAfter, 10)
	validBefore, _ := new(big.Int).SetString(authorization.ValidBefore, 10)
	nonceBytes, _ := evm.HexToBytes(authorization.Nonce)

	message := map[string]interface{}{
		"from":        authorization.From,
		"to":          authorization.To,
		"value":       value,
		"validAfter":  validAfter,
		"validBefore": validBefore,
		"nonce":       nonceBytes,
	}

	return c.signer.SignTypedData(ctx, domain, types, "TransferWithAuthorization", message)
}

// queryDomainSeparator queries DOMAIN_SEPARATOR from the token contract
func (c *ExactEvmScheme) queryDomainSeparator(ctx context.Context, tokenAddress string) ([]byte, error) {
	const domainSeparatorABI = `[{"constant":true,"inputs":[],"name":"DOMAIN_SEPARATOR","outputs":[{"internalType":"bytes32","name":"","type":"bytes32"}],"stateMutability":"view","type":"function"}]`

	contractABI, err := abi.JSON(strings.NewReader(domainSeparatorABI))
	if err != nil {
		return nil, err
	}

	addr := common.HexToAddress(tokenAddress)
	callData := contractABI.Methods["DOMAIN_SEPARATOR"].ID

	result, err := c.ethClient.CallContract(ctx, ethereum.CallMsg{
		To:   &addr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, err
	}

	if len(result) < 32 {
		return nil, fmt.Errorf("invalid DOMAIN_SEPARATOR result length: %d", len(result))
	}

	return result[:32], nil
}

// signWithDomainSeparator signs using the chain's DOMAIN_SEPARATOR directly
func (c *ExactEvmScheme) signWithDomainSeparator(
	ctx context.Context,
	authorization evm.ExactEIP3009Authorization,
	domainSeparator []byte,
) ([]byte, error) {
	// Use standard EIP-3009 typehash
	// TRANSFER_WITH_AUTHORIZATION_TYPEHASH = keccak256("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)")
	typeHash := crypto.Keccak256([]byte("TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"))

	// Parse values
	value, _ := new(big.Int).SetString(authorization.Value, 10)
	validAfter, _ := new(big.Int).SetString(authorization.ValidAfter, 10)
	validBefore, _ := new(big.Int).SetString(authorization.ValidBefore, 10)
	nonceBytes, _ := evm.HexToBytes(authorization.Nonce)
	fromAddr := common.HexToAddress(authorization.From)
	toAddr := common.HexToAddress(authorization.To)

	// Encode struct using ABI encoding: abi.encode(typeHash, from, to, value, validAfter, validBefore, nonce)
	// Manual encoding: each value is 32 bytes (ABI encoding pads to 32 bytes)
	// Build encoded data: typeHash (32) + from (32) + to (32) + value (32) + validAfter (32) + validBefore (32) + nonce (32)
	encoded := make([]byte, 0, 32*7)
	encoded = append(encoded, typeHash...)
	encoded = append(encoded, common.LeftPadBytes(fromAddr.Bytes(), 32)...)
	encoded = append(encoded, common.LeftPadBytes(toAddr.Bytes(), 32)...)
	encoded = append(encoded, common.LeftPadBytes(value.Bytes(), 32)...)
	encoded = append(encoded, common.LeftPadBytes(validAfter.Bytes(), 32)...)
	encoded = append(encoded, common.LeftPadBytes(validBefore.Bytes(), 32)...)
	encoded = append(encoded, nonceBytes...)

	structHash := crypto.Keccak256(encoded)

	// Build digest: keccak256(0x19 || 0x01 || domainSeparator || structHash)
	digest := crypto.Keccak256(
		append([]byte{0x19, 0x01},
			append(domainSeparator, structHash...)...,
		),
	)

	// Sign the digest directly
	return c.signDigest(ctx, digest)
}

// signDigest signs a raw digest (used when we have DOMAIN_SEPARATOR from chain)
func (c *ExactEvmScheme) signDigest(ctx context.Context, digest []byte) ([]byte, error) {
	return c.signer.SignDigest(ctx, digest)
}
