// exact + permit2 + BSC USDT demo (mainnet by default).
//
// Builds an x402 `scheme=exact` payload with `extra.assetTransferMethod=permit2`,
// and signs a `permit2Authorization` structure inside the EVM payload.
//
// Environment:
//   EVM_PRIVATE_KEY       - payer private key (hex, optional 0x)
//   PERMIT_SPENDER        - x402 exact permit2 proxy/spender address (goes into permit2Authorization.spender)
//   PAYEE_ADDRESS         - merchant payTo (goes into permit2Authorization.witness.to)
//   PAYMENT_AMOUNT        - amount in token smallest units (default 1000000000000000000 = 1 USDT at 18 decimals)
//   BSC_NETWORK           - "bsc" | "eip155:56" | "bsc-testnet" | "eip155:97" (default eip155:56)
//   USDT_ADDRESS          - optional override for USDT token; empty uses SDK default for the network
//   PERMIT_NONCE          - optional permit2Authorization nonce (default 0)
//   PERMIT_DEADLINE       - optional unix seconds deadline (default now+3600)
//   WITNESS_VALID_AFTER  - optional witness validAfter (default 0)
//
// Run from this directory:
//
//	go run .
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	x402 "github.com/gatechain/x402/go"
	evm "github.com/gatechain/x402/go/mechanisms/evm"
	exactevmclient "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	x402evmsigner "github.com/gatechain/x402/go/signers/evm"
	"github.com/gatechain/x402/go/types"
)

func main() {
	key := os.Getenv("EVM_PRIVATE_KEY")
	if key == "" {
		log.Fatal("EVM_PRIVATE_KEY required")
	}
	spender := os.Getenv("PERMIT_SPENDER")
	if spender == "" {
		log.Fatal("PERMIT_SPENDER required (permit2Authorization.spender)")
	}
	payTo := os.Getenv("PAYEE_ADDRESS")
	if payTo == "" {
		log.Fatal("PAYEE_ADDRESS required (permit2Authorization.witness.to)")
	}

	amount := os.Getenv("PAYMENT_AMOUNT")
	if amount == "" {
		amount = "1000000000000000000" // 1 USDT at 18 decimals (BSC mainnet default USDT in SDK)
	}

	network := os.Getenv("BSC_NETWORK")
	if network == "" {
		network = "eip155:56"
	}

	signer, err := x402evmsigner.NewClientSignerFromPrivateKey(key)
	if err != nil {
		log.Fatal(err)
	}

	net := x402.Network(network)
	client := x402.Newx402Client().
		Register(net, exactevmclient.NewExactEvmScheme(signer))

	asset := os.Getenv("USDT_ADDRESS")
	signExtra := map[string]interface{}{
		"assetTransferMethod": "permit2",
		// used by ExactEvmScheme.createPermit2Payload to override spender
		"spender": spender,
	}
	if v := os.Getenv("PERMIT_NONCE"); v != "" {
		signExtra["permitNonce"] = v
	}
	if v := os.Getenv("PERMIT_DEADLINE"); v != "" {
		signExtra["deadline"] = v
	}
	if v := os.Getenv("WITNESS_VALID_AFTER"); v != "" {
		signExtra["validAfter"] = v
	}

	req := types.PaymentRequirements{
		Scheme:            evm.SchemeExact,
		Network:           network,
		Asset:             asset,
		Amount:            amount,
		PayTo:             payTo,
		MaxTimeoutSeconds: 300,
		Extra:             signExtra,
	}

	payload, err := client.CreatePaymentPayload(context.Background(), req, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Match your requested settlement shape:
	// keep paymentRequirements.extra minimal, while the permit2Authorization fields
	// inside paymentPayload already contain the concrete nonce/deadline/witness values.
	req.Extra = map[string]interface{}{
		"assetTransferMethod": "permit2",
	}

	request := map[string]interface{}{
		"x402Version":           2,
		"paymentPayload":       payload,
		"paymentRequirements": req,
	}

	out, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(out))
}
