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
	"strconv"
	"strings"
	"time"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evm "github.com/gatechain/x402/go/mechanisms/evm"
	exactclient "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	exactserver "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
	evmsigner "github.com/gatechain/x402/go/signers/evm"
	"github.com/gatechain/x402/go/types"
	"github.com/gin-gonic/gin"
)

type facilitatorAPIResponse[T any] struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data T      `json:"data"`
}

const (
	// Same style as examples/go/servers/gate_weather:
	// Gate Web3 OpenAPI test facilitator (action: x402.supported/verify/settle)
	defaultFacilitatorURL = "https://openapi-test.gateweb3.cc/api/v1/x402"

	// User-provided BSC Permit2 proxy (x402Permit2Proxy)
	// Updated to match openapi-test exact+permit2 verification expectations.
	defaultPermit2ProxySpender = "0x701cCFfcdE34b92B16599Ac865AA1395A1a1F38c"

	// Default payment: 0.0001 USDT on BSC, assuming 18 decimals (BEP20 USDT).
	// If your USDT uses 6 decimals, override PAYMENT_AMOUNT accordingly.
	defaultPaymentAmount = "100000000000000"

	// You can override this via EVM_PAYEE_ADDRESS.
	// Leaving it as a placeholder avoids forcing config for quick local runs,
	// but you SHOULD set it to your actual merchant address.
	defaultEvmPayeeAddress = "0x000000000000000000000000000000000000dEaD"
)

func getenvDefault(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}

func getenvDefaultInt64(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("invalid %s: %v", name, err)
	}
	return n
}

func main() {
	// -----------------------------
	// Config (server side)
	// -----------------------------
	facilitatorURL := getenvDefault("FACILITATOR_URL", defaultFacilitatorURL)

	// If you only want to test the wiring/signature -> verify/settle flow without
	// any on-chain transaction, you can enable a local mock facilitator.
	//
	// In this mode:
	// - Set MOCK_FACILITATOR=1
	// - You can leave FACILITATOR_URL empty
	// - settle() returns dummy tx hash
	mockFacilitator := os.Getenv("MOCK_FACILITATOR") != ""
	if mockFacilitator {
		if facilitatorURL == "" {
			facilitatorURL = "http://localhost:4022"
		}
		startMockFacilitator(facilitatorURL)
	}

	if facilitatorURL == "" {
		log.Fatal("FACILITATOR_URL required (same as other Go examples)")
	}

	evmPayee := getenvDefault("EVM_PAYEE_ADDRESS", defaultEvmPayeeAddress)
	if evmPayee == "" {
		log.Fatal("EVM_PAYEE_ADDRESS required (merchant payTo / witness.to)")
	}

	proxySpender := getenvDefault("PERMIT_SPENDER", defaultPermit2ProxySpender)
	if proxySpender == "" {
		log.Fatal("PERMIT_SPENDER required (x402Permit2Proxy address; used as permit2Authorization.spender)")
	}

	permitNonce := getenvDefaultInt64("PERMIT_NONCE", 0)
	witnessValidAfter := getenvDefaultInt64("WITNESS_VALID_AFTER", 0)
	permitDeadline := getenvDefaultInt64("PERMIT_DEADLINE", time.Now().Add(1*time.Hour).Unix())

	paymentAmount := getenvDefault("PAYMENT_AMOUNT", defaultPaymentAmount)

	usdtAddr := os.Getenv("USDT_ADDRESS")
	if usdtAddr == "" {
		assetInfo, err := evm.GetAssetInfo("bsc", "")
		if err != nil {
			log.Fatalf("failed to get default USDT address: %v", err)
		}
		usdtAddr = assetInfo.Address
	}

	serverAddr := getenvDefault("SERVER_ADDR", ":4023")
	serverBaseURL := getenvDefault("SERVER_BASE_URL", "http://localhost:4023")

	const routePath = "/pay"
	// Gate facilitator uses "bsc" (not CAIP-2 eip155:56) for BSC mainnet.
	const network = "bsc"

	// -----------------------------
	// Build x402 server
	// -----------------------------
	// NOTE: middleware ignores PaymentOptions.Extra, so we must put permit2 fields into
	// price["extra"] to ensure they end up in paymentRequirements.extra.
	extra := map[string]interface{}{
		"assetTransferMethod": "permit2",
		"spender":             proxySpender,
		"permitNonce":         fmt.Sprintf("%d", permitNonce),
		"deadline":            fmt.Sprintf("%d", permitDeadline),
		"validAfter":          fmt.Sprintf("%d", witnessValidAfter),
	}

	price := map[string]interface{}{
		"amount": paymentAmount,
		"asset":  usdtAddr,
		"extra":  extra,
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: facilitatorURL,
	})

	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes: x402http.RoutesConfig{
			"GET " + routePath: {
				Accepts: x402http.PaymentOptions{
					{
						Scheme:  "exact",
						Price:   price,
						Network: network,
						PayTo:   evmPayee,
					},
				},
				Description: "permit2-style exact payment (BSC mainnet USDT)",
				MimeType:    "application/json",
			},
		},
		Facilitator: facilitatorClient,
		Schemes: []ginmw.SchemeConfig{
			{Network: network, Server: exactserver.NewExactEvmScheme()},
		},
		Timeout: 30 * time.Second,
	}))

	r.GET(routePath, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"route":   routePath,
			"network": network,
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	srv := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	// Start server in background goroutine (as requested)
	go func() {
		log.Printf("server starting on %s (facilitator=%s)", serverAddr, facilitatorURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	waitForHealthy(serverBaseURL + "/health")

	log.Printf("server ready. Try curl (expect 402 + PAYMENT-REQUIRED):")
	log.Printf("  curl -i %s%s", serverBaseURL, routePath)

	// Optional: run built-in client flow in the same process
	if os.Getenv("RUN_CLIENT") != "" {
		if err := runClientFlow(serverBaseURL+routePath, network); err != nil {
			log.Fatalf("client flow failed: %v", err)
		}
	}

	// Keep running so you can curl from another terminal.
	select {}
}

func waitForHealthy(url string) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	log.Printf("warning: server health check not ready yet at %s", url)
}

func startMockFacilitator(baseURL string) {
	// baseURL example: http://localhost:4022
	addr := strings.TrimPrefix(baseURL, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	handler := func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)

		action, _ := req["action"].(string)
		params, _ := req["params"].(map[string]interface{})

		// Always return Gate OpenAPI-like envelope: {code, msg, data}
		w.Header().Set("Content-Type", "application/json")

		switch action {
		case "x402.supported":
			resp := facilitatorAPIResponse[x402.SupportedResponse]{
				Code: 0, Msg: "",
				Data: x402.SupportedResponse{
					Kinds: []x402.SupportedKind{
						{X402Version: 2, Scheme: "exact", Network: "bsc"},
					},
					Extensions: []string{},
					Signers:    map[string][]string{},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "x402.verify":
			resp := facilitatorAPIResponse[x402.VerifyResponse]{
				Code: 0, Msg: "",
				Data: x402.VerifyResponse{IsValid: true, Payer: "0xmock"},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case "x402.settle":
			// Try to extract network/payment requirements
			var networkStr string
			if pr, ok := params["paymentRequirements"].(map[string]interface{}); ok {
				if n, ok := pr["network"].(string); ok {
					networkStr = n
				}
			}
			if networkStr == "" {
				networkStr = "bsc"
			}

			resp := facilitatorAPIResponse[x402.SettleResponse]{
				Code: 0, Msg: "",
				Data: x402.SettleResponse{
					Success:     true,
					Transaction: "0xmocksettledtx",
					Network:     x402.Network(networkStr),
					Payer:       "0xmock",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			// Unknown action
			resp := facilitatorAPIResponse[map[string]interface{}]{
				Code: 1, Msg: "mock_facilitator_unknown_action",
				Data: map[string]interface{}{},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}

	go func() {
		log.Printf("mock facilitator starting on http://%s", addr)
		if err := http.ListenAndServe(addr, http.HandlerFunc(handler)); err != nil && err != http.ErrServerClosed {
			log.Fatalf("mock facilitator failed: %v", err)
		}
	}()

	// Make sure the facilitator is reachable before x402 middleware initialization.
	healthReqBody := `{"action":"x402.supported","params":{}}`
	for i := 0; i < 40; i++ { // ~2s max
		_, err := http.Post(baseURL, "application/json", strings.NewReader(healthReqBody))
		if err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("warning: mock facilitator may not be ready yet (%s)", baseURL)
}

func runClientFlow(serverURL string, network string) error {
	key := os.Getenv("EVM_PRIVATE_KEY")
	if key == "" {
		return fmt.Errorf("EVM_PRIVATE_KEY required for RUN_CLIENT")
	}

	signer, err := evmsigner.NewClientSignerFromPrivateKey(key)
	if err != nil {
		return err
	}

	c := x402.Newx402Client().Register(x402.Network(network), exactclient.NewExactEvmScheme(signer))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req1, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return err
	}
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		return err
	}
	defer resp1.Body.Close()

	body1, _ := io.ReadAll(resp1.Body)
	if resp1.StatusCode != http.StatusPaymentRequired && resp1.StatusCode != 402 {
		return fmt.Errorf("expected 402, got %d; body=%s", resp1.StatusCode, string(body1))
	}

	requiredHeader := resp1.Header.Get("PAYMENT-REQUIRED")
	if requiredHeader == "" {
		for k, v := range resp1.Header {
			if strings.EqualFold(k, "PAYMENT-REQUIRED") && len(v) > 0 {
				requiredHeader = v[0]
				break
			}
		}
	}
	if requiredHeader == "" {
		return fmt.Errorf("missing PAYMENT-REQUIRED header")
	}

	requiredBytes, err := base64.StdEncoding.DecodeString(requiredHeader)
	if err != nil {
		return err
	}

	var paymentRequired x402.PaymentRequired
	if err := json.Unmarshal(requiredBytes, &paymentRequired); err != nil {
		return err
	}
	if len(paymentRequired.Accepts) == 0 {
		return fmt.Errorf("paymentRequired.accepts is empty")
	}

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
		return fmt.Errorf("no accepted requirement found for scheme=exact network=%s", network)
	}

	paymentPayload, err := c.CreatePaymentPayload(ctx, selected, nil, nil)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(paymentPayload)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(payloadBytes)

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return err
	}
	req2.Header.Set("PAYMENT-SIGNATURE", encoded)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	log.Printf("client retry status=%d body=%s", resp2.StatusCode, string(body2))
	if resp2.StatusCode == http.StatusPaymentRequired || resp2.StatusCode == 402 {
		// Dump a few high-signal headers to see why we got a payment error.
		log.Printf("client retry headers: PAYMENT-REQUIRED=%t PAYMENT-RESPONSE=%t Content-Type=%s",
			resp2.Header.Get("PAYMENT-REQUIRED") != "",
			resp2.Header.Get("PAYMENT-RESPONSE") != "",
			resp2.Header.Get("Content-Type"),
		)
	}

	if settleHeader := resp2.Header.Get("PAYMENT-RESPONSE"); settleHeader != "" {
		decoded, err := base64.StdEncoding.DecodeString(settleHeader)
		if err == nil {
			var settleResp x402.SettleResponse
			if err := json.Unmarshal(decoded, &settleResp); err == nil {
				log.Printf("settle: success=%v tx=%s network=%s payer=%s errorReason=%s",
					settleResp.Success, settleResp.Transaction, settleResp.Network, settleResp.Payer, settleResp.ErrorReason)
			}
		}
	}

	if required2 := resp2.Header.Get("PAYMENT-REQUIRED"); required2 != "" {
		decoded, err := base64.StdEncoding.DecodeString(required2)
		if err == nil {
			var pr x402.PaymentRequired
			if err := json.Unmarshal(decoded, &pr); err == nil {
				if pr.Error != "" {
					log.Printf("payment-required error: %s", pr.Error)
				}
			}
		}
	}
	return nil
}
