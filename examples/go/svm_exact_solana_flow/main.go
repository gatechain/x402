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
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	svm "github.com/gatechain/x402/go/mechanisms/svm"
	svmclient "github.com/gatechain/x402/go/mechanisms/svm/exact/client"
	svmserver "github.com/gatechain/x402/go/mechanisms/svm/exact/server"
	svmsigner "github.com/gatechain/x402/go/signers/svm"
	"github.com/gin-gonic/gin"
	"github.com/gatechain/x402/go/types"
	"github.com/joho/godotenv"
)

const defaultFacilitatorURL = "https://openapi-test.gateweb3.cc/api/v1/x402"
const defaultOpenApiTestDevnetMint = "BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3"

func getenvDefault(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}

func normalizeSolanaNetworkForGateOpenapi(network string) string {
	// Gate openapi-test /supported currently returns Solana networks as V1 names
	// (e.g. "solana-devnet"), and verify rejects CAIP-2 strings.
	switch strings.TrimSpace(network) {
	case svm.SolanaMainnetCAIP2:
		return svm.SolanaMainnetV1
	case svm.SolanaDevnetCAIP2:
		return svm.SolanaDevnetV1
	case svm.SolanaTestnetCAIP2:
		return svm.SolanaTestnetV1
	default:
		return network
	}
}

func main() {
	_ = godotenv.Load()

	// Shared settings
	port := getenvDefault("PORT", "4024")
	routePath := getenvDefault("ROUTE_PATH", "/pay")
	serverURL := "http://localhost:" + port + routePath

	networkInput := getenvDefault("SVM_NETWORK", svm.SolanaDevnetV1)
	networkGate := normalizeSolanaNetworkForGateOpenapi(networkInput)
	// Validate network is something our SVM mechanism understands (V1 or CAIP-2).
	if _, err := svm.NormalizeNetwork(networkGate); err != nil {
		log.Fatalf("invalid SVM_NETWORK=%s: %v", networkInput, err)
	}

	// === Server side (resource server) ===
	facilitatorURL := getenvDefault("FACILITATOR_URL", defaultFacilitatorURL)
	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{URL: facilitatorURL})

	payTo := strings.TrimSpace(getenvDefault("SVM_PAYEE_ADDRESS", ""))
	if payTo == "" {
		log.Fatal("SVM_PAYEE_ADDRESS required (Solana recipient pubkey, base58)")
	}

	assetMint := getenvDefault("SVM_ASSET_MINT", defaultOpenApiTestDevnetMint)
	amountAtomic := getenvDefault("PAYMENT_AMOUNT_ATOMIC", "100000") // 0.10 if decimals=6

	price := map[string]interface{}{
		"amount": amountAtomic,
		"asset":  assetMint,
	}

	routes := x402http.RoutesConfig{
		"GET " + routePath: {
			Accepts: []x402http.PaymentOption{
				{
					Scheme:  "exact",
					Network: x402.Network(networkGate),
					PayTo:   payTo,
					Price:   price,
				},
			},
			Description: "x402 exact payment on Solana (SVM)",
			MimeType:    "application/json",
		},
	}

	svmExactServer := svmserver.NewExactSvmScheme()

	app := gin.New()
	app.Use(gin.Recovery())
	app.Use(ginmw.X402Payment(ginmw.Config{
		Routes:                 routes,
		Facilitator:            facilitator,
		SyncFacilitatorOnStart: true,
		Schemes: []ginmw.SchemeConfig{
			{Network: x402.Network(networkGate), Server: svmExactServer},
		},
	}))

	app.GET(routePath, func(c *gin.Context) {
		c.JSON(200, gin.H{
			"ok":      true,
			"route":   routePath,
			"network": networkGate,
			"asset":   assetMint,
			"amount":  amountAtomic,
		})
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: app,
	}

	go func() {
		log.Printf("Go SVM x402 server listening %s", serverURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Small delay to avoid racing server startup
	time.Sleep(150 * time.Millisecond)

	// === Client side (payer) ===
	privateKey := strings.TrimSpace(getenvDefault("SVM_CLIENT_PRIVATE_KEY", ""))
	if privateKey == "" {
		log.Fatal("SVM_CLIENT_PRIVATE_KEY required (Solana private key, base58)")
	}

	signer, err := svmsigner.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		log.Fatal(err)
	}

	client := x402.Newx402Client().
		Register(x402.Network(networkGate), svmclient.NewExactSvmScheme(signer))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := runClient(ctx, serverURL, networkGate, client); err != nil {
		log.Printf("client error: %v", err)
	}

	if strings.TrimSpace(getenvDefault("KEEP_ALIVE", "")) == "1" {
		select {}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

type paymentPayloadCreator interface {
	CreatePaymentPayload(
		ctx context.Context,
		requirements types.PaymentRequirements,
		resource *types.ResourceInfo,
		extensions map[string]interface{},
	) (types.PaymentPayload, error)
}

func runClient(ctx context.Context, serverURL string, networkCAIP2 string, client paymentPayloadCreator) error {
	// Phase 1
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
	log.Printf("client initial status=%d body=%s", resp1.StatusCode, string(body1))
	if resp1.StatusCode != http.StatusPaymentRequired && resp1.StatusCode != 402 {
		return nil
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
		return fmt.Errorf("no accepted requirement found for scheme=exact network=%s", networkCAIP2)
	}
	log.Printf("selected: scheme=%s network=%s asset=%s amount=%s payTo=%s extra=%v",
		selected.Scheme, selected.Network, selected.Asset, selected.Amount, selected.PayTo, selected.Extra)

	// Phase 2
	paymentPayload, err := client.CreatePaymentPayload(ctx, selected, paymentRequired.Resource, paymentRequired.Extensions)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(paymentPayload)
	if err != nil {
		return err
	}
	paymentHeader := base64.StdEncoding.EncodeToString(payloadBytes)

	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL, nil)
	if err != nil {
		return err
	}
	req2.Header.Set("PAYMENT-SIGNATURE", paymentHeader)

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return err
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
	return nil
}

