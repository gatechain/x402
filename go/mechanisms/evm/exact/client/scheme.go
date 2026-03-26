package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
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

const permit2CanonicalAddress = "0x000000000022D473030F116dDEE9F6B43aC78BA3"

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

	// Permit2 path for exact scheme (x402 exact + assetTransferMethod=permit2)
	if requirements.Extra != nil {
		if method, ok := requirements.Extra["assetTransferMethod"].(string); ok && strings.EqualFold(method, "permit2") {
			return c.createPermit2Payload(ctx, requirements, chainID, assetInfo, value)
		}
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

	// For gatelayer_testnet with known token, use hardcoded DOMAIN_SEPARATOR from chain
	if networkStr == "gatelayer_testnet" {
		if sepHex, ok := evm.GateLayerTestnetDomainSeparators[strings.ToLower(assetInfo.Address)]; ok {
			domainSeparator, err := hex.DecodeString(strings.TrimPrefix(sepHex, "0x"))
			if err == nil && len(domainSeparator) == 32 {
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

func (c *ExactEvmScheme) createPermit2Payload(
	ctx context.Context,
	requirements types.PaymentRequirements,
	chainID *big.Int,
	assetInfo *evm.AssetInfo,
	amount *big.Int,
) (types.PaymentPayload, error) {
	spender := requirements.PayTo
	if requirements.Extra != nil {
		if v, ok := requirements.Extra["permit2Spender"].(string); ok && evm.IsValidAddress(v) {
			spender = v
		} else if v, ok := requirements.Extra["spender"].(string); ok && evm.IsValidAddress(v) {
			spender = v
		}
	}
	if !evm.IsValidAddress(spender) {
		return types.PaymentPayload{}, fmt.Errorf("invalid permit2 spender: %s", spender)
	}
	if !evm.IsValidAddress(requirements.PayTo) {
		return types.PaymentPayload{}, fmt.Errorf("invalid payTo: %s", requirements.PayTo)
	}

	nonce := big.NewInt(0)
	deadline := big.NewInt(time.Now().Unix() + 3600)
	validAfter := big.NewInt(0)
	if requirements.Extra != nil {
		if v, ok := extraBigInt(requirements.Extra, "permitNonce"); ok {
			nonce = v
		} else if v, ok := extraBigInt(requirements.Extra, "nonce"); ok {
			nonce = v
		}
		if v, ok := extraBigInt(requirements.Extra, "deadline"); ok {
			deadline = v
		}
		if v, ok := extraBigInt(requirements.Extra, "validAfter"); ok {
			validAfter = v
		}
	}

	// Signature must be valid for Permit2.permitWitnessTransferFrom().
	// Permit2 uses a domain separator without EIP712 "version" field, so we can't rely on SignTypedData.
	// Instead we compute the exact digest following PermitHash.sol + Permit2's EIP712 implementation.
	//
	// References:
	// - Uniswap permit2 PermitHash.hashWithWitness()
	// - Uniswap permit2 EIP712._hashTypedData()
	//
	// Permit2 witness type (matches x402-facilitator/examples/permit2_client default):
	//   Witness witness
	//   TokenPermissions(address token,uint256 amount)
	//   Witness(address to,uint256 validAfter)
	witnessTypeString := "Witness witness)TokenPermissions(address token,uint256 amount)Witness(address to,uint256 validAfter)"
	witnessTypeHash := crypto.Keccak256([]byte("Witness(address to,uint256 validAfter)"))

	// witnessHash = keccak256(abi.encode(witnessTypeHash, witness.to, witness.validAfter))
	witnessTo := common.HexToAddress(requirements.PayTo)
	witnessHashEnc := make([]byte, 0, 32*3)
	witnessHashEnc = append(witnessHashEnc, witnessTypeHash...)
	witnessHashEnc = append(witnessHashEnc, common.LeftPadBytes(witnessTo.Bytes(), 32)...)
	witnessHashEnc = append(witnessHashEnc, common.LeftPadBytes(validAfter.Bytes(), 32)...)
	witnessHash := crypto.Keccak256(witnessHashEnc)

	// tokenPermissionsHash = keccak256(abi.encode(TOKEN_PERMISSIONS_TYPEHASH, token, amount))
	tokenPermissionsTypehash := crypto.Keccak256([]byte("TokenPermissions(address token,uint256 amount)"))
	tokenAddr := common.HexToAddress(assetInfo.Address)
	tokenPermissionsEnc := make([]byte, 0, 32*3)
	tokenPermissionsEnc = append(tokenPermissionsEnc, tokenPermissionsTypehash...)
	tokenPermissionsEnc = append(tokenPermissionsEnc, common.LeftPadBytes(tokenAddr.Bytes(), 32)...)
	tokenPermissionsEnc = append(tokenPermissionsEnc, common.LeftPadBytes(amount.Bytes(), 32)...)
	tokenPermissionsHash := crypto.Keccak256(tokenPermissionsEnc)

	// typeHash = keccak256(abi.encodePacked(PERMIT_TRANSFER_FROM_WITNESS_TYPEHASH_STUB, witnessTypeString))
	permitTransferFromWitnessTypeHashStub := "PermitWitnessTransferFrom(TokenPermissions permitted,address spender,uint256 nonce,uint256 deadline,"
	typeHash := crypto.Keccak256([]byte(permitTransferFromWitnessTypeHashStub + witnessTypeString))

	// Permit hash: keccak256(abi.encode(typeHash, tokenPermissionsHash, msg.sender, nonce, deadline, witnessHash))
	//
	// Here msg.sender is the external caller of permitWitnessTransferFrom on Permit2.
	// For x402 this is the x402Permit2Proxy address, which is `spender` in permit2Authorization.
	spenderAddr := common.HexToAddress(spender)
	permitHashEnc := make([]byte, 0, 32*6)
	permitHashEnc = append(permitHashEnc, typeHash...)
	permitHashEnc = append(permitHashEnc, tokenPermissionsHash...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(spenderAddr.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(nonce.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(deadline.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, witnessHash...)
	permitHash := crypto.Keccak256(permitHashEnc)

	// Domain separator:
	// keccak256(abi.encode(EIP712DomainTypeHash, keccak256("Permit2"), chainId, verifyingContract))
	// verifyingContract is the canonical Permit2 contract address.
	domainTypeHash := crypto.Keccak256([]byte("EIP712Domain(string name,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256([]byte("Permit2"))
	verifyingContractAddr := common.HexToAddress(permit2CanonicalAddress)

	domainEnc := make([]byte, 0, 32*4)
	domainEnc = append(domainEnc, domainTypeHash...)
	domainEnc = append(domainEnc, nameHash...)
	domainEnc = append(domainEnc, common.LeftPadBytes(chainID.Bytes(), 32)...)
	domainEnc = append(domainEnc, common.LeftPadBytes(verifyingContractAddr.Bytes(), 32)...)
	domainSeparator := crypto.Keccak256(domainEnc)

	// digest = keccak256(0x19 0x01 || domainSeparator || permitHash)
	digestRaw := make([]byte, 0, 2+32+32)
	digestRaw = append(digestRaw, 0x19, 0x01)
	digestRaw = append(digestRaw, domainSeparator...)
	digestRaw = append(digestRaw, permitHash...)
	digest := crypto.Keccak256(digestRaw)

	sig, err := c.signer.SignDigest(ctx, digest)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("failed to sign permit2 witness authorization: %w", err)
	}

	return types.PaymentPayload{
		X402Version: 2,
		Payload: map[string]interface{}{
			"signature": evm.BytesToHex(sig),
			"permit2Authorization": map[string]interface{}{
				"from":    c.signer.Address(),
				"spender": spender,
				"permitted": map[string]interface{}{
					"token":  assetInfo.Address,
					"amount": amount.String(),
				},
				"nonce":    nonce.String(),
				"deadline": deadline.String(),
				"witness": map[string]interface{}{
					"to":         requirements.PayTo,
					"validAfter": validAfter.String(),
				},
			},
		},
	}, nil
}

func extraBigInt(extra map[string]interface{}, key string) (*big.Int, bool) {
	raw, ok := extra[key]
	if !ok || raw == nil {
		return nil, false
	}

	switch v := raw.(type) {
	case string:
		bi, ok := new(big.Int).SetString(v, 10)
		return bi, ok
	case int:
		return big.NewInt(int64(v)), true
	case int32:
		return big.NewInt(int64(v)), true
	case int64:
		return big.NewInt(v), true
	case uint:
		return new(big.Int).SetUint64(uint64(v)), true
	case uint32:
		return new(big.Int).SetUint64(uint64(v)), true
	case uint64:
		return new(big.Int).SetUint64(v), true
	case float64:
		if v == float64(int64(v)) {
			return big.NewInt(int64(v)), true
		}
	case float32:
		f := float64(v)
		if f == float64(int64(f)) {
			return big.NewInt(int64(f)), true
		}
	default:
		if s, ok := raw.(fmt.Stringer); ok {
			bi, ok := new(big.Int).SetString(s.String(), 10)
			return bi, ok
		}
		if n, err := strconv.ParseInt(fmt.Sprintf("%v", raw), 10, 64); err == nil {
			return big.NewInt(n), true
		}
	}

	return nil, false
}
