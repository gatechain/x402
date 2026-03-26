package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evm "github.com/gatechain/x402/go/mechanisms/evm"
	evm_exact_server "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
	"github.com/gin-gonic/gin"
)

const defaultFacilitatorURL = "https://openapi-test.gateweb3.cc/api/v1/x402"
const defaultPermit2ProxySpender = "0x701cCFfcdE34b92B16599Ac865AA1395A1a1F38c"

func getenvDefault(name, def string) string {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	return v
}

func getenvDefaultInt(name string, def int64) int64 {
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
	// Note: this server does NOT directly sign anything.
	// It returns x402 PAYMENT-REQUIRED responses. The client signs and retries with PAYMENT-SIGNATURE.
	//
	// Run:
	//   FACILITATOR_URL=http://localhost:4022/ go run .
	//   Then run the client example in another terminal.

	facilitatorURL := getenvDefault("FACILITATOR_URL", defaultFacilitatorURL)

	evmPayee := os.Getenv("EVM_PAYEE_ADDRESS")
	if evmPayee == "" {
		log.Fatal("EVM_PAYEE_ADDRESS required (merchant payTo / witness.to)")
	}

	proxySpender := os.Getenv("PERMIT_SPENDER")
	if proxySpender == "" {
		proxySpender = defaultPermit2ProxySpender
	}

	// Permit2 witness timing
	permitNonce := getenvDefaultInt("PERMIT_NONCE", 0)
	validAfter := getenvDefaultInt("WITNESS_VALID_AFTER", 0)
	deadline := getenvDefaultInt("PERMIT_DEADLINE", time.Now().Add(1*time.Hour).Unix())

	// Payment amount (smallest unit). Default is 1 USDT on BSC mainnet (18 decimals).
	paymentAmount := os.Getenv("PAYMENT_AMOUNT")
	if paymentAmount == "" {
		paymentAmount = "1000000000000000000"
	}

	// USDT address (default to SDK BSC mainnet USDT)
	usdtAddr := os.Getenv("USDT_ADDRESS")
	if usdtAddr == "" {
		assetInfo, err := evm.GetAssetInfo("eip155:56", "")
		if err != nil {
			log.Fatalf("failed to get default USDT address: %v", err)
		}
		usdtAddr = assetInfo.Address
	}

	// Server route
	const routePath = "/pay"
	const network = "eip155:56"

	// NOTE: x402 exact scheme ParsePrice expects AssetAmount as a map.
	price := map[string]interface{}{
		"amount": paymentAmount,
		"asset":  usdtAddr,
	}

	// Provider-supplied fields needed for the client to compute permit2Authorization + signature.
	// These values must match what the client will sign.
	extra := map[string]interface{}{
		"assetTransferMethod": "permit2",
		"spender":             proxySpender,
		"permitNonce":         fmt.Sprintf("%d", permitNonce),
		"deadline":            fmt.Sprintf("%d", deadline),
		"validAfter":          fmt.Sprintf("%d", validAfter),
	}

	// NOTE: middleware currently ignores PaymentOptions.Extra, so we must put
	// permit2 parameters into price["extra"] for them to reach paymentRequirements.extra.
	price["extra"] = extra

	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: facilitatorURL,
	})

	// Create Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

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
				Description: "permit2-style exact payment (server-client demo)",
				MimeType:    "application/json",
			},
		},
		Facilitator: facilitatorClient,
		Schemes: []ginmw.SchemeConfig{
			{
				Network: network,
				Server:  evm_exact_server.NewExactEvmScheme(),
			},
		},
		Timeout: 30 * time.Second,
	}))

	r.GET(routePath, func(c *gin.Context) {
		// If we reach here, the payment has already been verified + settled by middleware.
		c.JSON(http.StatusOK, gin.H{
			"ok":      true,
			"route":   routePath,
			"network": network,
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":4023"
	}

	log.Printf("server listening on http://localhost%s%s", addr, routePath)

	if err := r.Run(addr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
