package main

import (
	"log"
	"os"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	svm "github.com/gatechain/x402/go/mechanisms/svm"
	svmserver "github.com/gatechain/x402/go/mechanisms/svm/exact/server"
	"github.com/gin-gonic/gin"
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

func main() {
	_ = godotenv.Load()

	// Server basics
	port := getenvDefault("PORT", "4024")
	routePath := getenvDefault("ROUTE_PATH", "/pay")
	serverURL := "http://localhost:" + port + routePath

	// x402 / facilitator
	facilitatorURL := getenvDefault("FACILITATOR_URL", defaultFacilitatorURL)
	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{URL: facilitatorURL})

	// Solana network (CAIP-2). Defaults to devnet.
	networkInput := getenvDefault("SVM_NETWORK", svm.SolanaDevnetV1) // allow "solana-devnet"
	networkStr, err := svm.NormalizeNetwork(networkInput)
	if err != nil {
		log.Fatalf("invalid SVM_NETWORK=%s: %v", networkInput, err)
	}
	network := x402.Network(networkStr)

	// Merchant "payTo" (Solana recipient address). REQUIRED.
	payTo := stringsTrim(getenvDefault("SVM_PAYEE_ADDRESS", ""))
	if payTo == "" {
		log.Fatal("SVM_PAYEE_ADDRESS required (Solana recipient pubkey, base58)")
	}

	// Token mint + atomic amount (smallest unit).
	// Default mint is the OpenAPI-test devnet token you provided.
	assetMint := getenvDefault("SVM_ASSET_MINT", defaultOpenApiTestDevnetMint)
	amountAtomic := getenvDefault("PAYMENT_AMOUNT_ATOMIC", "100000") // 0.10 USDC if decimals=6

	// IMPORTANT: SVM exact ParsePrice expects AssetAmount as a map.
	price := map[string]interface{}{
		"amount": amountAtomic,
		"asset":  assetMint,
	}

	// Routes config
	routes := x402http.RoutesConfig{
		"GET " + routePath: {
			Accepts: []x402http.PaymentOption{
				{
					Scheme:  "exact",
					Network: network,
					PayTo:   payTo,
					Price:   price,
					// maxTimeoutSeconds can be overridden later if needed
				},
			},
			Description: "x402 exact payment on Solana (SVM)",
			MimeType:    "application/json",
		},
	}

	// Register SVM exact scheme server. It will inject facilitator-provided fields (e.g. feePayer) into requirements.extra.
	svmExactServer := svmserver.NewExactSvmScheme()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes:                 routes,
		Facilitator:            facilitator,
		SyncFacilitatorOnStart: true,
		Schemes: []ginmw.SchemeConfig{
			{Network: network, Server: svmExactServer},
		},
	}))

	r.GET(routePath, func(c *gin.Context) {
		c.JSON(200, gin.H{
			"ok":      true,
			"route":   routePath,
			"network": networkStr, // CAIP-2
			"asset":   assetMint,
			"amount":  amountAtomic,
		})
	})

	log.Printf("Go SVM x402 server listening %s", serverURL)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		i := len(s) - 1
		if s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == '\r' {
			s = s[:i]
			continue
		}
		break
	}
	return s
}

