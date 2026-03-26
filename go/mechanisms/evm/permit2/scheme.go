package permit2

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/gatechain/x402/go/mechanisms/evm"
	"github.com/gatechain/x402/go/types"
)

// Permit2EvmScheme implements x402 SchemeNetworkClient for Permit2 PermitSingle signatures.
// Facilitators that support scheme "permit2" can verify the EIP-712 signature and call Permit2.permit + transferFrom.
//
// PaymentRequirements expectations:
//   - network: e.g. "eip155:97", "bsc-testnet", "eip155:56", "bsc"
//   - asset: token address (USDT on BSC testnet by default from NetworkConfigs if empty)
//   - amount: smallest unit string (e.g. wei for 18-decimal USDT)
//   - payTo: spender address (who is allowed to pull tokens via Permit2, typically facilitator/router)
//
// Optional requirements.Extra:
//   - "permitNonce" (string or float64): current allowance nonce for (owner, token, spender) on Permit2; default "0"
//   - "expiration" (string): unix seconds when allowance expires; default now + 30 days
//   - "sigDeadline" (string): unix seconds EIP-712 deadline; default now + 1 hour
type Permit2EvmScheme struct {
	signer evm.ClientEvmSigner
}

// NewPermit2EvmScheme creates a client scheme that signs PermitSingle typed data for Permit2.
func NewPermit2EvmScheme(signer evm.ClientEvmSigner) *Permit2EvmScheme {
	return &Permit2EvmScheme{signer: signer}
}

// Scheme returns "permit2".
func (p *Permit2EvmScheme) Scheme() string {
	return SchemePermit2
}

// CreatePaymentPayload builds a V2 payload with permitSingle + signature for Gate / custom facilitators.
func (p *Permit2EvmScheme) CreatePaymentPayload(
	ctx context.Context,
	requirements types.PaymentRequirements,
) (types.PaymentPayload, error) {
	networkStr := string(requirements.Network)

	chainID, err := evm.GetEvmChainId(networkStr)
	if err != nil {
		return types.PaymentPayload{}, err
	}

	assetInfo, err := evm.GetAssetInfo(networkStr, requirements.Asset)
	if err != nil {
		return types.PaymentPayload{}, err
	}

	amount, ok := new(big.Int).SetString(requirements.Amount, 10)
	if !ok {
		return types.PaymentPayload{}, fmt.Errorf("invalid amount: %s", requirements.Amount)
	}

	if !evm.IsValidAddress(requirements.PayTo) {
		return types.PaymentPayload{}, fmt.Errorf("invalid payTo (spender): %s", requirements.PayTo)
	}

	tokenAddr := common.HexToAddress(assetInfo.Address)
	spenderAddr := common.HexToAddress(requirements.PayTo)

	now := time.Now().Unix()
	expiration := big.NewInt(now + 30*86400)
	sigDeadline := big.NewInt(now + 3600)
	nonce := big.NewInt(0)

	if requirements.Extra != nil {
		if v, ok := extraBigInt(requirements.Extra, "expiration"); ok {
			expiration = v
		}
		if v, ok := extraBigInt(requirements.Extra, "sigDeadline"); ok {
			sigDeadline = v
		}
		if v, ok := extraBigInt(requirements.Extra, "permitNonce"); ok {
			nonce = v
		}
	}

	domain := evm.TypedDataDomain{
		Name:              "Permit2",
		Version:           "1",
		ChainID:           chainID,
		VerifyingContract: Permit2Address,
	}

	message := map[string]interface{}{
		"details": map[string]interface{}{
			"token":      tokenAddr.Hex(),
			"amount":     amount,
			"expiration": expiration,
			"nonce":      nonce,
		},
		"spender":     spenderAddr.Hex(),
		"sigDeadline": sigDeadline,
	}

	typesDef := PermitSingleTypes()
	sig, err := p.signer.SignTypedData(ctx, domain, typesDef, "PermitSingle", message)
	if err != nil {
		return types.PaymentPayload{}, fmt.Errorf("permit2 SignTypedData: %w", err)
	}

	payload := map[string]interface{}{
		"owner": p.signer.Address(),
		"permitSingle": map[string]interface{}{
			"details": map[string]interface{}{
				"token":      tokenAddr.Hex(),
				"amount":     amount.String(),
				"expiration": expiration.String(),
				"nonce":      nonce.String(),
			},
			"spender":     spenderAddr.Hex(),
			"sigDeadline": sigDeadline.String(),
		},
		"signature": evm.BytesToHex(sig),
	}

	return types.PaymentPayload{
		X402Version: 2,
		Payload:     payload,
	}, nil
}

func extraBigInt(extra map[string]interface{}, key string) (*big.Int, bool) {
	v, ok := extra[key]
	if !ok || v == nil {
		return nil, false
	}
	switch t := v.(type) {
	case string:
		n, ok := new(big.Int).SetString(t, 10)
		return n, ok
	case float64:
		return big.NewInt(int64(t)), true
	case int:
		return big.NewInt(int64(t)), true
	case int64:
		return big.NewInt(t), true
	default:
		return nil, false
	}
}
