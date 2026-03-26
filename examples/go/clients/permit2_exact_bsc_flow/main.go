package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	x402 "github.com/gatechain/x402/go"
	exactclient "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	evmsigner "github.com/gatechain/x402/go/signers/evm"
	"github.com/gatechain/x402/go/types"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4023/pay"
	}

	key := os.Getenv("EVM_PRIVATE_KEY")
	if key == "" {
		log.Fatal("EVM_PRIVATE_KEY required")
	}

	network := os.Getenv("BSC_NETWORK")
	if network == "" {
		network = "eip155:56"
	}

	// Build x402 client (client-side EIP-712 signature + payload generation)
	signer, err := evmsigner.NewClientSignerFromPrivateKey(key)
	if err != nil {
		log.Fatal(err)
	}

	c := x402.Newx402Client().Register(x402.Network(network), exactclient.NewExactEvmScheme(signer))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Phase 1: request payment required
	req1, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		log.Fatal(err)
	}
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		log.Fatal(err)
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != http.StatusPaymentRequired && resp1.StatusCode != 402 {
		log.Fatalf("expected 402 Payment Required, got %d; body=%s", resp1.StatusCode, string(body1))
	}

	requiredHeader := resp1.Header.Get("PAYMENT-REQUIRED")
	if requiredHeader == "" {
		// Be tolerant to header casing differences
		for k, v := range resp1.Header {
			if strings.EqualFold(k, "PAYMENT-REQUIRED") && len(v) > 0 {
				requiredHeader = v[0]
				break
			}
		}
	}
	if requiredHeader == "" {
		log.Fatalf("missing PAYMENT-REQUIRED header; body=%s", string(body1))
	}

	requiredBytes, err := base64.StdEncoding.DecodeString(requiredHeader)
	if err != nil {
		log.Fatal(err)
	}

	var paymentRequired x402.PaymentRequired
	if err := json.Unmarshal(requiredBytes, &paymentRequired); err != nil {
		log.Fatal(err)
	}

	if len(paymentRequired.Accepts) == 0 {
		log.Fatal("paymentRequired.accepts is empty")
	}

	// Pick the exact payment option (scheme + network)
	var selected types.PaymentRequirements
	found := false
	for _, acc := range paymentRequired.Accepts {
		if strings.EqualFold(acc.Scheme, "exact") && acc.Network == network {
			selected = acc
			found = true
			break
		}
	}
	if !found {
		log.Fatalf("no accepted requirement found for scheme=exact network=%s", network)
	}

	log.Printf("Selected payment requirements: scheme=%s network=%s asset=%s amount=%s payTo=%s extra=%v",
		selected.Scheme, selected.Network, selected.Asset, selected.Amount, selected.PayTo, selected.Extra)

	// Phase 2: client signs & builds payment payload for the server
	paymentPayload, err := c.CreatePaymentPayload(ctx, selected, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	encodedPayloadBytes, err := json.Marshal(paymentPayload)
	if err != nil {
		log.Fatal(err)
	}

	encoded := base64.StdEncoding.EncodeToString(encodedPayloadBytes)

	// Phase 3: retry with PAYMENT-SIGNATURE header
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		log.Fatal(err)
	}
	req2.Header.Set("PAYMENT-SIGNATURE", encoded)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	log.Printf("Second response status=%d body=%s", resp2.StatusCode, string(body2))

	settleHeader := resp2.Header.Get("PAYMENT-RESPONSE")
	if settleHeader != "" {
		log.Printf("PAYMENT-RESPONSE header (base64)=%s", settleHeader)
	} else {
		log.Printf("No PAYMENT-RESPONSE header")
	}

	// Optional: pretty print payment payload if needed
	if os.Getenv("PRINT_PAYLOAD") != "" {
		out, _ := json.MarshalIndent(paymentPayload, "", "  ")
		fmt.Println(string(out))
	}
}

