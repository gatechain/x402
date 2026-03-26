package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	x402 "github.com/gatechain/x402/go"
	svm "github.com/gatechain/x402/go/mechanisms/svm"
	svmclient "github.com/gatechain/x402/go/mechanisms/svm/exact/client"
	svmsigner "github.com/gatechain/x402/go/signers/svm"
	"github.com/gatechain/x402/go/types"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:4024/pay"
	}

	network := os.Getenv("SVM_NETWORK")
	if network == "" {
		network = svm.SolanaDevnetV1 // allow "solana-devnet" by default
	}
	networkCAIP2, err := svm.NormalizeNetwork(network)
	if err != nil {
		log.Fatalf("invalid SVM_NETWORK=%s: %v", network, err)
	}

	// Client signer (payer)
	privateKey := os.Getenv("SVM_CLIENT_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatal("SVM_CLIENT_PRIVATE_KEY required (Solana private key, base58)")
	}

	signer, err := svmsigner.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		log.Fatal(err)
	}

	// Build x402 client and register SVM exact scheme on the chosen network
	c := x402.Newx402Client().
		Register(x402.Network(networkCAIP2), svmclient.NewExactSvmScheme(signer))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Pick exact+network requirement
	var selected types.PaymentRequirements
	found := false
	for _, acc := range paymentRequired.Accepts {
		if strings.EqualFold(acc.Scheme, "exact") && acc.Network == networkCAIP2 {
			selected = acc
			found = true
			break
		}
	}
	if !found {
		log.Fatalf("no accepted requirement found for scheme=exact network=%s; accepts=%v", networkCAIP2, paymentRequired.Accepts)
	}

	log.Printf("Selected payment requirements: scheme=%s network=%s asset=%s amount=%s payTo=%s extra=%v",
		selected.Scheme, selected.Network, selected.Asset, selected.Amount, selected.PayTo, selected.Extra)

	// Phase 2: build payment payload (client signs a Solana tx, facilitator will co-sign/settle)
	paymentPayload, err := c.CreatePaymentPayload(ctx, selected, paymentRequired.Resource, paymentRequired.Extensions)
	if err != nil {
		log.Fatal(err)
	}

	payloadBytes, err := json.Marshal(paymentPayload)
	if err != nil {
		log.Fatal(err)
	}
	paymentHeader := base64.StdEncoding.EncodeToString(payloadBytes)

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		log.Fatal(err)
	}
	req2.Header.Set("PAYMENT-SIGNATURE", paymentHeader)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	log.Printf("retry status=%d body=%s", resp2.StatusCode, string(body2))
	if pr := resp2.Header.Get("PAYMENT-REQUIRED"); pr != "" {
		decoded, _ := base64.StdEncoding.DecodeString(pr)
		log.Printf("retry PAYMENT-REQUIRED=%s", string(decoded))
	}
	if ps := resp2.Header.Get("PAYMENT-RESPONSE"); ps != "" {
		decoded, _ := base64.StdEncoding.DecodeString(ps)
		log.Printf("PAYMENT-RESPONSE=%s", string(decoded))
	}
}

