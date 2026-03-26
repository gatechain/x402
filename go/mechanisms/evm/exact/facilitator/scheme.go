package facilitator

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	x402 "github.com/gatechain/x402/go"
	"github.com/gatechain/x402/go/mechanisms/evm"
	"github.com/gatechain/x402/go/types"
)

const permit2CanonicalAddress = "0x000000000022D473030F116dDEE9F6B43aC78BA3"

// ExactEvmSchemeConfig holds configuration for the ExactEvmScheme facilitator
type ExactEvmSchemeConfig struct {
	// DeployERC4337WithEIP6492 enables automatic deployment of ERC-4337 smart wallets
	// via EIP-6492 when encountering undeployed contract signatures during settlement
	DeployERC4337WithEIP6492 bool
}

// ExactEvmScheme implements the SchemeNetworkFacilitator interface for EVM exact payments (V2)
type ExactEvmScheme struct {
	signer evm.FacilitatorEvmSigner
	config ExactEvmSchemeConfig
}

// NewExactEvmScheme creates a new ExactEvmScheme
// Args:
//
//	signer: The EVM signer for facilitator operations
//	config: Optional configuration (nil uses defaults)
//
// Returns:
//
//	Configured ExactEvmScheme instance
func NewExactEvmScheme(signer evm.FacilitatorEvmSigner, config *ExactEvmSchemeConfig) *ExactEvmScheme {
	cfg := ExactEvmSchemeConfig{}
	if config != nil {
		cfg = *config
	}
	return &ExactEvmScheme{
		signer: signer,
		config: cfg,
	}
}

// Scheme returns the scheme identifier
func (f *ExactEvmScheme) Scheme() string {
	return evm.SchemeExact
}

// CaipFamily returns the CAIP family pattern this facilitator supports
func (f *ExactEvmScheme) CaipFamily() string {
	return "eip155:*"
}

// GetExtra returns mechanism-specific extra data for the supported kinds endpoint.
// For EVM, no extra data is needed.
func (f *ExactEvmScheme) GetExtra(_ x402.Network) map[string]interface{} {
	return nil
}

// GetSigners returns signer addresses used by this facilitator.
// Returns all addresses this facilitator can use for signing/settling transactions.
func (f *ExactEvmScheme) GetSigners(_ x402.Network) []string {
	return f.signer.GetAddresses()
}

// Verify verifies a V2 payment payload against requirements
func (f *ExactEvmScheme) Verify(
	ctx context.Context,
	payload types.PaymentPayload,
	requirements types.PaymentRequirements,
) (*x402.VerifyResponse, error) {
	network := x402.Network(requirements.Network)

	// Validate scheme (v2 has scheme in Accepted field)
	if payload.Accepted.Scheme != evm.SchemeExact {
		return nil, x402.NewVerifyError(ErrInvalidScheme, "", network, nil)
	}

	// Validate network (v2 has network in Accepted field)
	if payload.Accepted.Network != requirements.Network {
		return nil, x402.NewVerifyError(ErrNetworkMismatch, "", network, nil)
	}

	// Permit2 asset transfer method for exact scheme
	if requirements.Extra != nil {
		if method, ok := requirements.Extra["assetTransferMethod"].(string); ok && strings.EqualFold(method, "permit2") {
			return f.verifyPermit2(ctx, payload, requirements)
		}
	}

	// Parse EVM payload
	evmPayload, err := evm.PayloadFromMap(payload.Payload)
	if err != nil {
		return nil, x402.NewVerifyError(ErrInvalidPayload, "", network, err)
	}

	// Validate signature exists
	if evmPayload.Signature == "" {
		return nil, x402.NewVerifyError(ErrMissingSignature, "", network, nil)
	}

	// Get network configuration
	networkStr := string(requirements.Network)
	config, err := evm.GetNetworkConfig(networkStr)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetNetworkConfig, "", network, err)
	}

	// Get asset info
	assetInfo, err := evm.GetAssetInfo(networkStr, requirements.Asset)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetAssetInfo, "", network, err)
	}

	// Validate authorization matches requirements
	if !strings.EqualFold(evmPayload.Authorization.To, requirements.PayTo) {
		return nil, x402.NewVerifyError(ErrRecipientMismatch, "", network, nil)
	}

	// Parse and validate amount
	authValue, ok := new(big.Int).SetString(evmPayload.Authorization.Value, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidAuthorizationValue, "", network, nil)
	}

	// Requirements.Amount is already in the smallest unit
	requiredValue, ok := new(big.Int).SetString(requirements.Amount, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidRequiredAmount, "", network, fmt.Errorf("invalid amount: %s", requirements.Amount))
	}

	if authValue.Cmp(requiredValue) < 0 {
		return nil, x402.NewVerifyError(ErrInsufficientAmount, evmPayload.Authorization.From, network, nil)
	}

	// Check if nonce has been used
	nonceUsed, err := f.checkNonceUsed(ctx, evmPayload.Authorization.From, evmPayload.Authorization.Nonce, assetInfo.Address)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToCheckNonce, evmPayload.Authorization.From, network, err)
	}
	if nonceUsed {
		return nil, x402.NewVerifyError(ErrNonceAlreadyUsed, evmPayload.Authorization.From, network, nil)
	}

	// Check balance
	balance, err := f.signer.GetBalance(ctx, evmPayload.Authorization.From, assetInfo.Address)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetBalance, evmPayload.Authorization.From, network, err)
	}
	if balance.Cmp(authValue) < 0 {
		return nil, x402.NewVerifyError(ErrInsufficientBalance, evmPayload.Authorization.From, network, nil)
	}

	// Extract token info from requirements
	tokenName := assetInfo.Name
	tokenVersion := assetInfo.Version
	if requirements.Extra != nil {
		if name, ok := requirements.Extra["name"].(string); ok {
			tokenName = name
		}
		if version, ok := requirements.Extra["version"].(string); ok {
			tokenVersion = version
		}
	}

	// Verify signature
	signatureBytes, err := evm.HexToBytes(evmPayload.Signature)
	if err != nil {
		return nil, x402.NewVerifyError(ErrInvalidSignatureFormat, evmPayload.Authorization.From, network, err)
	}

	valid, err := f.verifySignature(
		ctx,
		evmPayload.Authorization,
		signatureBytes,
		config.ChainID,
		assetInfo.Address,
		tokenName,
		tokenVersion,
	)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToVerifySignature, evmPayload.Authorization.From, network, err)
	}

	if !valid {
		return nil, x402.NewVerifyError(ErrInvalidSignature, evmPayload.Authorization.From, network, nil)
	}

	return &x402.VerifyResponse{
		IsValid: true,
		Payer:   evmPayload.Authorization.From,
	}, nil
}

// Settle settles a V2 payment on-chain
func (f *ExactEvmScheme) Settle(
	ctx context.Context,
	payload types.PaymentPayload,
	requirements types.PaymentRequirements,
) (*x402.SettleResponse, error) {
	network := x402.Network(payload.Accepted.Network)

	// First verify the payment
	verifyResp, err := f.Verify(ctx, payload, requirements)
	if err != nil {
		// Convert VerifyError to SettleError
		ve := &x402.VerifyError{}
		if errors.As(err, &ve) {
			return nil, x402.NewSettleError(ve.Reason, ve.Payer, ve.Network, "", ve.Err)
		}
		return nil, x402.NewSettleError(ErrVerificationFailed, "", network, "", err)
	}

	// Permit2 settlement for exact scheme
	if requirements.Extra != nil {
		if method, ok := requirements.Extra["assetTransferMethod"].(string); ok && strings.EqualFold(method, "permit2") {
			return f.settlePermit2(ctx, payload, requirements, verifyResp)
		}
	}

	// Parse EVM payload
	evmPayload, err := evm.PayloadFromMap(payload.Payload)
	if err != nil {
		return nil, x402.NewSettleError(ErrInvalidPayload, verifyResp.Payer, network, "", err)
	}

	// Get asset info
	networkStr := string(requirements.Network)
	assetInfo, err := evm.GetAssetInfo(networkStr, requirements.Asset)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToGetAssetInfo, verifyResp.Payer, network, "", err)
	}

	// Parse signature
	signatureBytes, err := evm.HexToBytes(evmPayload.Signature)
	if err != nil {
		return nil, x402.NewSettleError(ErrInvalidSignatureFormat, verifyResp.Payer, network, "", err)
	}

	// Parse ERC-6492 signature to extract inner signature if needed
	sigData, err := evm.ParseERC6492Signature(signatureBytes)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToParseSignature, verifyResp.Payer, network, "", err)
	}

	// Check if wallet needs deployment (undeployed smart wallet with ERC-6492)
	zeroFactory := [20]byte{}
	if sigData.Factory != zeroFactory && len(sigData.FactoryCalldata) > 0 {
		code, err := f.signer.GetCode(ctx, evmPayload.Authorization.From)
		if err != nil {
			return nil, x402.NewSettleError(ErrFailedToCheckDeployment, verifyResp.Payer, network, "", err)
		}

		if len(code) == 0 {
			// Wallet not deployed
			if f.config.DeployERC4337WithEIP6492 {
				// Deploy wallet
				err := f.deploySmartWallet(ctx, sigData)
				if err != nil {
					return nil, x402.NewSettleError(evm.ErrSmartWalletDeploymentFailed, verifyResp.Payer, network, "", err)
				}
			} else {
				// Deployment not enabled - fail settlement
				return nil, x402.NewSettleError(evm.ErrUndeployedSmartWallet, verifyResp.Payer, network, "", nil)
			}
		}
	}

	// Use inner signature for settlement
	signatureBytes = sigData.InnerSignature

	// Parse values
	value, _ := new(big.Int).SetString(evmPayload.Authorization.Value, 10)
	validAfter, _ := new(big.Int).SetString(evmPayload.Authorization.ValidAfter, 10)
	validBefore, _ := new(big.Int).SetString(evmPayload.Authorization.ValidBefore, 10)
	nonceBytes, _ := evm.HexToBytes(evmPayload.Authorization.Nonce)

	// Determine signature type: ECDSA (65 bytes) or smart wallet (longer)
	isECDSA := len(signatureBytes) == 65

	var txHash string
	if isECDSA {
		// For EOA wallets, use v,r,s overload
		r := signatureBytes[0:32]
		s := signatureBytes[32:64]
		v := signatureBytes[64]
		if v == 0 || v == 1 {
			v += 27
		}

		txHash, err = f.signer.WriteContract(
			ctx,
			assetInfo.Address,
			evm.TransferWithAuthorizationVRSABI,
			evm.FunctionTransferWithAuthorization,
			common.HexToAddress(evmPayload.Authorization.From),
			common.HexToAddress(evmPayload.Authorization.To),
			value,
			validAfter,
			validBefore,
			[32]byte(nonceBytes),
			v,
			[32]byte(r),
			[32]byte(s),
		)
	} else {
		// For smart wallets, use bytes signature overload
		txHash, err = f.signer.WriteContract(
			ctx,
			assetInfo.Address,
			evm.TransferWithAuthorizationBytesABI,
			evm.FunctionTransferWithAuthorization,
			common.HexToAddress(evmPayload.Authorization.From),
			common.HexToAddress(evmPayload.Authorization.To),
			value,
			validAfter,
			validBefore,
			[32]byte(nonceBytes),
			signatureBytes,
		)
	}

	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToExecuteTransfer, verifyResp.Payer, network, "", err)
	}

	// Wait for transaction confirmation
	receipt, err := f.signer.WaitForTransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToGetReceipt, verifyResp.Payer, network, txHash, err)
	}

	if receipt.Status != evm.TxStatusSuccess {
		return nil, x402.NewSettleError(ErrTransactionFailed, verifyResp.Payer, network, txHash, nil)
	}

	return &x402.SettleResponse{
		Success:     true,
		Transaction: txHash,
		Network:     network,
		Payer:       verifyResp.Payer,
	}, nil
}

// deploySmartWallet deploys an ERC-4337 smart wallet using the ERC-6492 factory
//
// This function sends the pre-encoded factory calldata directly as a transaction.
// The factoryCalldata already contains the complete encoded function call with selector.
//
// Args:
//
//	ctx: Context for cancellation
//	sigData: Parsed ERC-6492 signature containing factory address and calldata
//
// Returns:
//
//	error if deployment fails
func (f *ExactEvmScheme) deploySmartWallet(
	ctx context.Context,
	sigData *evm.ERC6492SignatureData,
) error {
	factoryAddr := common.BytesToAddress(sigData.Factory[:])

	// Send the factory calldata directly - it already contains the encoded function call
	txHash, err := f.signer.SendTransaction(
		ctx,
		factoryAddr.Hex(),
		sigData.FactoryCalldata,
	)
	if err != nil {
		return fmt.Errorf("factory deployment transaction failed: %w", err)
	}

	// Wait for deployment transaction
	receipt, err := f.signer.WaitForTransactionReceipt(ctx, txHash)
	if err != nil {
		return fmt.Errorf("failed to wait for deployment: %w", err)
	}

	if receipt.Status != evm.TxStatusSuccess {
		return fmt.Errorf("deployment transaction reverted")
	}

	return nil
}

// checkNonceUsed checks if a nonce has already been used
func (f *ExactEvmScheme) checkNonceUsed(ctx context.Context, from string, nonce string, tokenAddress string) (bool, error) {
	nonceBytes, err := evm.HexToBytes(nonce)
	if err != nil {
		return false, err
	}

	result, err := f.signer.ReadContract(
		ctx,
		tokenAddress,
		evm.AuthorizationStateABI,
		evm.FunctionAuthorizationState,
		common.HexToAddress(from),
		[32]byte(nonceBytes),
	)
	if err != nil {
		return false, err
	}

	used, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected result type from authorizationState")
	}

	return used, nil
}

// verifySignature verifies the EIP-712 signature
func (f *ExactEvmScheme) verifySignature(
	ctx context.Context,
	authorization evm.ExactEIP3009Authorization,
	signature []byte,
	chainID *big.Int,
	verifyingContract string,
	tokenName string,
	tokenVersion string,
) (bool, error) {
	// Hash the EIP-712 typed data
	hash, err := evm.HashEIP3009Authorization(
		authorization,
		chainID,
		verifyingContract,
		tokenName,
		tokenVersion,
	)
	if err != nil {
		return false, err
	}

	// Convert hash to [32]byte
	var hash32 [32]byte
	copy(hash32[:], hash)

	// Use universal verification (supports EOA, EIP-1271, and ERC-6492)
	valid, sigData, err := evm.VerifyUniversalSignature(
		ctx,
		f.signer,
		authorization.From,
		hash32,
		signature,
		true, // allowUndeployed in verify()
	)

	if err != nil {
		return false, err
	}

	// If undeployed wallet with deployment info, it will be deployed in settle()
	if sigData != nil {
		zeroFactory := [20]byte{}
		if sigData.Factory != zeroFactory {
			_, err := f.signer.GetCode(ctx, authorization.From)
			if err != nil {
				return false, err
			}
			// Wallet may not be deployed - this is OK in verify() if has deployment info
			// Actual deployment happens in settle() if configured
		}
	}

	return valid, nil
}

func (f *ExactEvmScheme) verifyPermit2(
	ctx context.Context,
	payload types.PaymentPayload,
	requirements types.PaymentRequirements,
) (*x402.VerifyResponse, error) {
	networkStr := string(requirements.Network)

	// Get chain ID - works for any EIP-155 network (eip155:CHAIN_ID)
	config, err := evm.GetNetworkConfig(networkStr)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetNetworkConfig, "", x402.Network(requirements.Network), err)
	}

	// Get asset info (token contract address, decimals, etc.)
	assetInfo, err := evm.GetAssetInfo(networkStr, requirements.Asset)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetAssetInfo, "", x402.Network(requirements.Network), err)
	}

	// Parse raw permit2 payload
	signatureHex, permitAuth, err := parsePermit2Authorization(payload.Payload)
	if err != nil {
		return nil, x402.NewVerifyError(ErrInvalidPayload, "", x402.Network(requirements.Network), err)
	}

	// Basic invariants
	if !strings.EqualFold(permitAuth.Witness.To, requirements.PayTo) {
		return nil, x402.NewVerifyError(ErrRecipientMismatch, "", x402.Network(requirements.Network), nil)
	}
	if !strings.EqualFold(permitAuth.Permitted.Token, assetInfo.Address) {
		return nil, x402.NewVerifyError(ErrFailedToGetAssetInfo, "", x402.Network(requirements.Network), fmt.Errorf("token mismatch: %s != %s", permitAuth.Permitted.Token, assetInfo.Address))
	}

	// Parse numeric fields
	permittedAmount, ok := new(big.Int).SetString(permitAuth.Permitted.Amount, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidAuthorizationValue, "", x402.Network(requirements.Network), fmt.Errorf("invalid permitted.amount: %s", permitAuth.Permitted.Amount))
	}
	requiredValue, ok := new(big.Int).SetString(requirements.Amount, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidRequiredAmount, "", x402.Network(requirements.Network), fmt.Errorf("invalid amount: %s", requirements.Amount))
	}
	if permittedAmount.Cmp(requiredValue) < 0 {
		return nil, x402.NewVerifyError(ErrInsufficientAmount, permitAuth.From, x402.Network(requirements.Network), nil)
	}

	nonce, ok := new(big.Int).SetString(permitAuth.Nonce, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidAuthorizationValue, permitAuth.From, x402.Network(requirements.Network), fmt.Errorf("invalid nonce: %s", permitAuth.Nonce))
	}
	deadline, ok := new(big.Int).SetString(permitAuth.Deadline, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidAuthorizationValue, permitAuth.From, x402.Network(requirements.Network), fmt.Errorf("invalid deadline: %s", permitAuth.Deadline))
	}
	validAfter, ok := new(big.Int).SetString(permitAuth.Witness.ValidAfter, 10)
	if !ok {
		return nil, x402.NewVerifyError(ErrInvalidAuthorizationValue, permitAuth.From, x402.Network(requirements.Network), fmt.Errorf("invalid witness.validAfter: %s", permitAuth.Witness.ValidAfter))
	}

	// Deadline / witness activity checks (fast-fail before signature verify)
	now := time.Now().Unix()
	if now > deadline.Int64() {
		return nil, x402.NewVerifyError("permit2_signature_expired", permitAuth.From, x402.Network(requirements.Network), nil)
	}
	if now < validAfter.Int64() {
		return nil, x402.NewVerifyError("permit2_witness_not_active", permitAuth.From, x402.Network(requirements.Network), nil)
	}

	// Verify signature
	signatureBytes, err := evm.HexToBytes(signatureHex)
	if err != nil {
		return nil, x402.NewVerifyError(ErrInvalidSignatureFormat, permitAuth.From, x402.Network(requirements.Network), err)
	}

	digest, err := computePermit2WitnessDigest(
		config.ChainID,
		permitAuth.Spender,
		permitAuth.Permitted.Token,
		permittedAmount,
		nonce,
		deadline,
		permitAuth.Witness.To,
		validAfter,
		[]byte{}, // witness.extra is empty by default for x402 permit2Authorization
	)
	if err != nil {
		return nil, x402.NewVerifyError("permit2_digest_failed", permitAuth.From, x402.Network(requirements.Network), err)
	}

	var hash32 [32]byte
	copy(hash32[:], digest)

	valid, sigData, err := evm.VerifyUniversalSignature(
		ctx,
		f.signer,
		permitAuth.From,
		hash32,
		signatureBytes,
		true, // allow undeployed smart wallets (deployment handled in settle)
	)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToVerifySignature, permitAuth.From, x402.Network(requirements.Network), err)
	}
	if sigData != nil {
		// If the wallet is undeployed, actual deployment is handled in settle() (optional).
	}
	if !valid {
		return nil, x402.NewVerifyError(ErrInvalidSignature, permitAuth.From, x402.Network(requirements.Network), nil)
	}

	// Check if Permit2 unordered nonce has been used
	nonceUsed, err := f.checkPermit2NonceUsed(ctx, permitAuth.From, nonce)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToCheckNonce, permitAuth.From, x402.Network(requirements.Network), err)
	}
	if nonceUsed {
		return nil, x402.NewVerifyError(ErrNonceAlreadyUsed, permitAuth.From, x402.Network(requirements.Network), nil)
	}

	// Check balance
	balance, err := f.signer.GetBalance(ctx, permitAuth.From, assetInfo.Address)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetBalance, permitAuth.From, x402.Network(requirements.Network), err)
	}
	if balance.Cmp(requiredValue) < 0 {
		return nil, x402.NewVerifyError(ErrInsufficientBalance, permitAuth.From, x402.Network(requirements.Network), nil)
	}

	// Check ERC20 allowance for Permit2 canonical contract
	allowance, err := f.checkPermit2Allowance(ctx, permitAuth.From, assetInfo.Address, permit2CanonicalAddress)
	if err != nil {
		return nil, x402.NewVerifyError(ErrFailedToGetBalance, permitAuth.From, x402.Network(requirements.Network), err)
	}
	if allowance.Cmp(requiredValue) < 0 {
		return nil, x402.NewVerifyError("permit2_allowance_required", permitAuth.From, x402.Network(requirements.Network), nil)
	}

	return &x402.VerifyResponse{
		IsValid: true,
		Payer:   permitAuth.From,
	}, nil
}

func (f *ExactEvmScheme) settlePermit2(
	ctx context.Context,
	payload types.PaymentPayload,
	requirements types.PaymentRequirements,
	verifyResp *x402.VerifyResponse,
) (*x402.SettleResponse, error) {
	network := x402.Network(payload.Accepted.Network)
	_ = string(requirements.Network)

	signatureHex, permitAuth, err := parsePermit2Authorization(payload.Payload)
	if err != nil {
		return nil, x402.NewSettleError(ErrInvalidPayload, verifyResp.Payer, network, "", err)
	}

	// Parse required amount
	requiredValue, ok := new(big.Int).SetString(requirements.Amount, 10)
	if !ok {
		return nil, x402.NewSettleError(ErrInvalidRequiredAmount, verifyResp.Payer, network, "", fmt.Errorf("invalid amount: %s", requirements.Amount))
	}

	// Parse signature
	signatureBytes, err := evm.HexToBytes(signatureHex)
	if err != nil {
		return nil, x402.NewSettleError(ErrInvalidSignatureFormat, verifyResp.Payer, network, "", err)
	}

	// Parse ERC-6492 signature to extract inner signature if needed
	sigData, err := evm.ParseERC6492Signature(signatureBytes)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToParseSignature, verifyResp.Payer, network, "", err)
	}

	// Check if wallet needs deployment (undeployed smart wallet with ERC-6492)
	zeroFactory := [20]byte{}
	if sigData.Factory != zeroFactory && len(sigData.FactoryCalldata) > 0 {
		code, err := f.signer.GetCode(ctx, permitAuth.From)
		if err != nil {
			return nil, x402.NewSettleError(ErrFailedToCheckDeployment, verifyResp.Payer, network, "", err)
		}

		if len(code) == 0 {
			// Wallet not deployed
			if f.config.DeployERC4337WithEIP6492 {
				if err := f.deploySmartWallet(ctx, sigData); err != nil {
					return nil, x402.NewSettleError(evm.ErrSmartWalletDeploymentFailed, verifyResp.Payer, network, "", err)
				}
			} else {
				return nil, x402.NewSettleError(evm.ErrUndeployedSmartWallet, verifyResp.Payer, network, "", nil)
			}
		}
	}

	// Use inner signature for settlement (Permit2 expects bytes signature format)
	signatureBytes = sigData.InnerSignature

	nonce, _ := new(big.Int).SetString(permitAuth.Nonce, 10)
	deadline, _ := new(big.Int).SetString(permitAuth.Deadline, 10)
	validAfter, _ := new(big.Int).SetString(permitAuth.Witness.ValidAfter, 10)
	permittedAmount, _ := new(big.Int).SetString(permitAuth.Permitted.Amount, 10)

	// Build permit and witness tuples for proxy.settle()
	type tokenPermissionsTuple struct {
		Token  common.Address
		Amount *big.Int
	}
	type permitTransferFromTuple struct {
		Permitted tokenPermissionsTuple
		Spender   common.Address
		Nonce     *big.Int
		Deadline  *big.Int
	}
	type witnessTuple struct {
		To         common.Address
		ValidAfter *big.Int
		Extra      []byte
	}

	permit := permitTransferFromTuple{
		Permitted: tokenPermissionsTuple{
			Token:  common.HexToAddress(permitAuth.Permitted.Token),
			Amount: permittedAmount,
		},
		Spender:  common.HexToAddress(permitAuth.Spender),
		Nonce:    nonce,
		Deadline: deadline,
	}
	witness := witnessTuple{
		To:         common.HexToAddress(permitAuth.Witness.To),
		ValidAfter: validAfter,
		Extra:      []byte{}, // witness.extra
	}

	proxyABI := x402Permit2ProxySettleABI
	proxyAddr := common.HexToAddress(permitAuth.Spender)

	txHash, err := f.signer.WriteContract(
		ctx,
		proxyAddr.Hex(),
		proxyABI,
		"settle",
		permit,
		requiredValue,
		common.HexToAddress(permitAuth.From),
		witness,
		signatureBytes,
	)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToExecuteTransfer, verifyResp.Payer, network, "", err)
	}

	receipt, err := f.signer.WaitForTransactionReceipt(ctx, txHash)
	if err != nil {
		return nil, x402.NewSettleError(ErrFailedToGetReceipt, verifyResp.Payer, network, txHash, err)
	}
	if receipt.Status != evm.TxStatusSuccess {
		return nil, x402.NewSettleError(ErrTransactionFailed, verifyResp.Payer, network, txHash, nil)
	}

	return &x402.SettleResponse{
		Success:     true,
		Transaction: txHash,
		Network:     network,
		Payer:       verifyResp.Payer,
	}, nil
}

type permit2AuthorizationParsed struct {
	From      string
	Spender   string
	Permitted struct {
		Token  string
		Amount string
	}
	Nonce    string
	Deadline string
	Witness  struct {
		To         string
		ValidAfter string
		Extra      interface{}
	}
}

func parsePermit2Authorization(payload map[string]interface{}) (signatureHex string, auth permit2AuthorizationParsed, err error) {
	sig, ok := payload["signature"].(string)
	if !ok || sig == "" {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing signature")
	}

	authRaw, ok := payload["permit2Authorization"].(map[string]interface{})
	if !ok {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization")
	}

	if from, ok := authRaw["from"].(string); ok {
		auth.From = from
	} else {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.from")
	}
	if spender, ok := authRaw["spender"].(string); ok {
		auth.Spender = spender
	} else {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.spender")
	}

	permittedRaw, ok := authRaw["permitted"].(map[string]interface{})
	if !ok {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.permitted")
	}
	if token, ok := permittedRaw["token"].(string); ok {
		auth.Permitted.Token = token
	} else {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.permitted.token")
	}
	if amt, ok := permittedRaw["amount"].(string); ok {
		auth.Permitted.Amount = amt
	} else {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.permitted.amount")
	}

	if nonce, ok := authRaw["nonce"].(string); ok {
		auth.Nonce = nonce
	} else {
		// Some encoders may use numbers instead of strings
		if nonceF, ok := authRaw["nonce"].(float64); ok {
			auth.Nonce = fmt.Sprintf("%.0f", nonceF)
		} else {
			return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.nonce")
		}
	}

	if deadline, ok := authRaw["deadline"].(string); ok {
		auth.Deadline = deadline
	} else {
		if deadlineF, ok := authRaw["deadline"].(float64); ok {
			auth.Deadline = fmt.Sprintf("%.0f", deadlineF)
		} else {
			return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.deadline")
		}
	}

	witnessRaw, ok := authRaw["witness"].(map[string]interface{})
	if !ok {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.witness")
	}
	if to, ok := witnessRaw["to"].(string); ok {
		auth.Witness.To = to
	} else {
		return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.witness.to")
	}
	if va, ok := witnessRaw["validAfter"].(string); ok {
		auth.Witness.ValidAfter = va
	} else {
		if vaF, ok := witnessRaw["validAfter"].(float64); ok {
			auth.Witness.ValidAfter = fmt.Sprintf("%.0f", vaF)
		} else {
			return "", permit2AuthorizationParsed{}, fmt.Errorf("missing permit2Authorization.witness.validAfter")
		}
	}

	// Optional extra is currently ignored by x402 client implementation.
	if extra, ok := witnessRaw["extra"]; ok {
		auth.Witness.Extra = extra
	}

	return sig, auth, nil
}

func computePermit2WitnessDigest(
	chainID *big.Int,
	proxySpender string,
	tokenAddr string,
	permittedAmount *big.Int,
	nonce *big.Int,
	deadline *big.Int,
	witnessTo string,
	validAfter *big.Int,
	witnessExtra []byte,
) ([]byte, error) {
	// Mirrors permit2 PermitHash.hashWithWitness() + permit2 EIP712._hashTypedData().
	// Mirrors Uniswap Permit2 witness hashing:
	//   WITNESS_TYPEHASH = keccak256("Witness(address to,uint256 validAfter)")
	//   witnessHash = keccak256(abi.encode(WITNESS_TYPEHASH, witness.to, witness.validAfter))
	//
	witnessTypeString := "Witness witness)TokenPermissions(address token,uint256 amount)Witness(address to,uint256 validAfter)"
	witnessTypeHash := crypto.Keccak256([]byte("Witness(address to,uint256 validAfter)"))

	// witnessHash = keccak256(abi.encode(witnessTypeHash, witness.to, witness.validAfter))
	witnessToAddr := common.HexToAddress(witnessTo)
	witnessHashEnc := make([]byte, 0, 32*3)
	witnessHashEnc = append(witnessHashEnc, witnessTypeHash...)
	witnessHashEnc = append(witnessHashEnc, common.LeftPadBytes(witnessToAddr.Bytes(), 32)...)
	witnessHashEnc = append(witnessHashEnc, common.LeftPadBytes(validAfter.Bytes(), 32)...)
	witnessHash := crypto.Keccak256(witnessHashEnc)

	// tokenPermissionsHash = keccak256(abi.encode(TokenPermissionsTypeHash, token, amount))
	tokenPermissionsTypehash := crypto.Keccak256([]byte("TokenPermissions(address token,uint256 amount)"))
	tokenAddrAddr := common.HexToAddress(tokenAddr)
	tokenPermissionsEnc := make([]byte, 0, 32*3)
	tokenPermissionsEnc = append(tokenPermissionsEnc, tokenPermissionsTypehash...)
	tokenPermissionsEnc = append(tokenPermissionsEnc, common.LeftPadBytes(tokenAddrAddr.Bytes(), 32)...)
	tokenPermissionsEnc = append(tokenPermissionsEnc, common.LeftPadBytes(permittedAmount.Bytes(), 32)...)
	tokenPermissionsHash := crypto.Keccak256(tokenPermissionsEnc)

	permitTransferFromWitnessTypeHashStub := "PermitWitnessTransferFrom(TokenPermissions permitted,address spender,uint256 nonce,uint256 deadline,"
	typeHash := crypto.Keccak256([]byte(permitTransferFromWitnessTypeHashStub + witnessTypeString))

	proxyAddr := common.HexToAddress(proxySpender)
	permitHashEnc := make([]byte, 0, 32*6)
	permitHashEnc = append(permitHashEnc, typeHash...)
	permitHashEnc = append(permitHashEnc, tokenPermissionsHash...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(proxyAddr.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(nonce.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, common.LeftPadBytes(deadline.Bytes(), 32)...)
	permitHashEnc = append(permitHashEnc, witnessHash...)
	permitHash := crypto.Keccak256(permitHashEnc)

	// EIP712 domain separator (permit2): EIP712Domain(string name,uint256 chainId,address verifyingContract)
	domainTypeHash := crypto.Keccak256([]byte("EIP712Domain(string name,uint256 chainId,address verifyingContract)"))
	nameHash := crypto.Keccak256([]byte("Permit2"))
	verifyingContractAddr := common.HexToAddress(permit2CanonicalAddress)

	domainEnc := make([]byte, 0, 32*4)
	domainEnc = append(domainEnc, domainTypeHash...)
	domainEnc = append(domainEnc, nameHash...)
	domainEnc = append(domainEnc, common.LeftPadBytes(chainID.Bytes(), 32)...)
	domainEnc = append(domainEnc, common.LeftPadBytes(verifyingContractAddr.Bytes(), 32)...)
	domainSeparator := crypto.Keccak256(domainEnc)

	digestRaw := make([]byte, 0, 2+32+32)
	digestRaw = append(digestRaw, 0x19, 0x01)
	digestRaw = append(digestRaw, domainSeparator...)
	digestRaw = append(digestRaw, permitHash...)
	digest := crypto.Keccak256(digestRaw)

	return digest, nil
}

func (f *ExactEvmScheme) checkPermit2NonceUsed(ctx context.Context, owner string, nonce *big.Int) (bool, error) {
	wordPos := new(big.Int).Rsh(new(big.Int).Set(nonce), 8)
	bitPos := new(big.Int).And(new(big.Int).Set(nonce), big.NewInt(255))

	nonceBitmapABI := []byte(`[
		{
			"inputs": [
				{"name":"owner","type":"address"},
				{"name":"wordPos","type":"uint256"}
			],
			"name":"nonceBitmap",
			"outputs": [{"name":"","type":"uint256"}],
			"stateMutability":"view",
			"type":"function"
		}
	]`)

	result, err := f.signer.ReadContract(
		ctx,
		permit2CanonicalAddress,
		nonceBitmapABI,
		"nonceBitmap",
		common.HexToAddress(owner),
		wordPos,
	)
	if err != nil {
		return false, err
	}

	usedWord, ok := result.(*big.Int)
	if !ok {
		// Some decoders might return big.Int directly
		if v, ok := result.(big.Int); ok {
			usedWord = &v
		} else {
			return false, fmt.Errorf("unexpected nonceBitmap return type: %T", result)
		}
	}

	mask := new(big.Int).Lsh(big.NewInt(1), uint(bitPos.Uint64()))
	and := new(big.Int).And(usedWord, mask)
	return and.Sign() != 0, nil
}

func (f *ExactEvmScheme) checkPermit2Allowance(
	ctx context.Context,
	owner string,
	tokenAddr string,
	spender string,
) (*big.Int, error) {
	allowanceABI := []byte(`[
		{
			"inputs": [
				{"name":"owner","type":"address"},
				{"name":"spender","type":"address"}
			],
			"name":"allowance",
			"outputs": [{"name":"","type":"uint256"}],
			"stateMutability":"view",
			"type":"function"
		}
	]`)

	result, err := f.signer.ReadContract(
		ctx,
		tokenAddr,
		allowanceABI,
		"allowance",
		common.HexToAddress(owner),
		common.HexToAddress(spender),
	)
	if err != nil {
		return nil, err
	}

	switch v := result.(type) {
	case *big.Int:
		return v, nil
	case big.Int:
		return &v, nil
	default:
		return nil, fmt.Errorf("unexpected allowance return type: %T", result)
	}
}

// ABI for x402Permit2Proxy.settle (reference implementation in specs).
var x402Permit2ProxySettleABI = []byte(`[
	{
		"inputs": [
			{
				"name": "permit",
				"type": "tuple",
				"components": [
					{
						"name": "permitted",
						"type": "tuple",
						"components": [
							{"name": "token", "type": "address"},
							{"name": "amount", "type": "uint256"}
						]
					},
					{"name": "spender", "type": "address"},
					{"name": "nonce", "type": "uint256"},
					{"name": "deadline", "type": "uint256"}
				]
			},
			{"name": "amount", "type": "uint256"},
			{"name": "owner", "type": "address"},
			{
				"name": "witness",
				"type": "tuple",
				"components": [
					{"name": "to", "type": "address"},
					{"name": "validAfter", "type": "uint256"},
					{"name": "extra", "type": "bytes"}
				]
			},
			{"name": "signature", "type": "bytes"}
		],
		"name": "settle",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`)
