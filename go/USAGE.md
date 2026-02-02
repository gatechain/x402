# x402 Go SDK Usage Guide

This guide walks you through how to use the **x402 Go SDK** to build payment-enabled applications. The SDK supports both **sellers** (servers that accept payments) and **buyers** (clients that make paid requests).

## Table of Contents

- [Quickstart for Sellers](#quickstart-for-sellers)
- [Quickstart for Buyers](#quickstart-for-buyers)
- [Configuration](#configuration)

---

## Quickstart for Sellers

This guide shows you how to integrate x402 into your Go server to accept payments for your API or service.

### Prerequisites

Before you begin, ensure you have:

* A crypto wallet to receive funds (any EVM-compatible wallet)
* [Go](https://go.dev/) 1.24+ installed
* An existing HTTP server (Gin, standard library, etc.)

### 1. Install Dependencies

Add the x402 Go module to your project:

```bash
go get github.com/gatechain/x402/go
go mod tidy
```

The `go mod tidy` command will automatically download all required dependencies and update your `go.mod` and `go.sum` files.

### 2. Set Up Environment Variables

Before running your server, set up the required environment variables:

```bash
# Required: Your wallet address to receive payments
export PAYEE_ADDRESS="0x1234567890123456789012345678901234567890"

# Required for Gate Web3 OpenAPI authentication
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"

# Optional
export GATE_WEB3_PASSPHRASE="your-passphrase"
export GATE_WEB3_REAL_IP="your-real-ip"
```

### 3. Create a Payment-Protected Server

Here's a complete, runnable example using Gin framework:

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evm "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
)

func main() {
	// Get receiving wallet address from environment variable
	payTo := os.Getenv("PAYEE_ADDRESS")
	if payTo == "" {
		fmt.Println("❌ PAYEE_ADDRESS environment variable is required")
		fmt.Println("   Example: export PAYEE_ADDRESS=0x1234567890123456789012345678901234567890")
		os.Exit(1)
	}

	network := x402.Network("gatelayer_testnet") // Gate Layer testnet

	fmt.Printf("🚀 Starting x402 payment server...\n")
	fmt.Printf("   Payee address: %s\n", payTo)
	fmt.Printf("   Network: %s\n", network)
	fmt.Printf("   Facilitator: https://openapi-test.gateweb3.cc/api/v1/x402\n\n")

	r := gin.Default()

	// Create facilitator client (Gate Web3 OpenAPI Testnet)
	// The client will automatically use Gate Web3 authentication if environment variables are set:
	// - GATE_WEB3_API_KEY
	// - GATE_WEB3_API_SECRET
	// - GATE_WEB3_PASSPHRASE (optional)
	// - GATE_WEB3_REAL_IP (optional)
	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	})

	// Apply x402 payment middleware
	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes: x402http.RoutesConfig{
			"GET /weather": {
				Accepts: x402http.PaymentOptions{
					{
						Scheme:  "exact",
						PayTo:   payTo,
						Price:   "$0.001", // Price in USD - automatically converts to USDC on the network
						Network: network,
					},
				},
				Description: "Get weather data for a city",
				MimeType:    "application/json",
			},
		},
		Facilitator: facilitatorClient,
		Schemes: []ginmw.SchemeConfig{
			{Network: network, Server: evm.NewExactEvmScheme()},
		},
		SyncFacilitatorOnStart: true,
		Timeout:                30 * time.Second,
	}))

	// Protected endpoint
	r.GET("/weather", func(c *gin.Context) {
		city := c.DefaultQuery("city", "San Francisco")
		c.JSON(http.StatusOK, gin.H{
			"city":        city,
			"weather":     "sunny",
			"temperature": 70,
			"timestamp":   time.Now().Format(time.RFC3339),
		})
	})

	// Health check endpoint (no payment required)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": "1.0.0",
		})
	})

	fmt.Printf("   Server listening on http://localhost:4021\n")
	if err := r.Run(":4021"); err != nil {
		fmt.Printf("❌ Error starting server: %v\n", err)
		os.Exit(1)
	}
}
```

### 4. Test Your Integration

1. Start your server:
   ```bash
   go run main.go
   ```

2. Make a request without payment:
   ```bash
   curl http://localhost:4021/weather
   ```

3. The server responds with `402 Payment Required` and payment instructions in the `PAYMENT-REQUIRED` header.

4. Use a compatible client (see [Quickstart for Buyers](#quickstart-for-buyers)) to complete the payment and retry the request.

5. After successful payment verification, the server returns your API response.

### 5. Payment Asset Selection

**How assets are selected:**

1. **Server-side (Seller)**: When you specify a price like `"$0.001"`, the SDK automatically:
   - Parses the USD amount
   - Looks up the default asset for the specified network (configured in `go/mechanisms/evm/constants.go`)
   - For `gatelayer_testnet`, the default asset is USDC at address `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`
   - For `gatelayer` (mainnet), the default asset is configured per network
   - Converts the USD amount to the token's smallest unit (e.g., $0.001 = 1000 for USDC with 6 decimals)
   - The SDK uses the chain's DOMAIN_SEPARATOR for signing to ensure compatibility with the token contract

2. **Client-side (Buyer)**: When the client receives payment requirements:
   - The client filters available options to those matching registered schemes/networks
   - The client uses the asset address specified in the payment requirements to create the payment

**Default Assets by Network:**

| Network | Default Asset | Address |
|---------|--------------|---------|
| `gatelayer_testnet` | USDC | `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` |
| `gatelayer` | USDC | (configured per network) |

**Note**: The asset is automatically determined from the network configuration. You don't need to specify the asset address when using USD pricing (`"$0.001"`).

**EIP-712 Signing**: The SDK automatically uses the chain's DOMAIN_SEPARATOR for signing. For `gatelayer_testnet`, it uses the correct DOMAIN_SEPARATOR from the token contract to ensure signatures are valid.

---

## Quickstart for Buyers

This guide shows you how to create a Go client that can make paid requests to x402-protected resources.

### Prerequisites

Before you begin, ensure you have:

* A crypto wallet with USDC (any EVM-compatible wallet)
* [Go](https://go.dev/) 1.24+ installed
* A service that requires payment via x402

### 1. Install Dependencies

Add the x402 Go module to your project:

```bash
go get github.com/gatechain/x402/go
go mod tidy
```

The `go mod tidy` command will automatically download all required dependencies and update your `go.mod` and `go.sum` files.

### 2. Create a Payment-Enabled HTTP Client

The SDK automatically handles payment creation and signing using the chain's DOMAIN_SEPARATOR. For `gatelayer_testnet`, it uses the correct DOMAIN_SEPARATOR from the token contract to ensure signatures are valid.

Here's a complete, runnable example:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	evm "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/gatechain/x402/go/signers/evm"
)

func main() {
	// Get configuration from environment
	privateKey := os.Getenv("EVM_PRIVATE_KEY")
	if privateKey == "" {
		fmt.Println("❌ EVM_PRIVATE_KEY environment variable is required")
		os.Exit(1)
	}

	url := os.Getenv("SERVER_URL")
	if url == "" {
		url = "http://localhost:4021/weather"
	}

	fmt.Printf("🚀 Making paid request to: %s\n\n", url)

	// Create EVM signer from private key
	evmSigner, err := evmsigners.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		fmt.Printf("❌ Failed to create signer: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Signer created: %s\n\n", evmSigner.Address())

	// Create x402 client and register EVM scheme
	// The SDK automatically uses the chain's DOMAIN_SEPARATOR for signing
	// For gatelayer_testnet, it uses the correct DOMAIN_SEPARATOR from the chain
	x402Client := x402.Newx402Client().
		Register("gatelayer_testnet", evm.NewExactEvmScheme(evmSigner))

	// Wrap HTTP client with payment handling
	// PaymentRoundTripper automatically handles 402 responses and retries with payment
	httpClient := x402http.WrapHTTPClientWithPayment(
		http.DefaultClient,
		x402http.Newx402HTTPClient(x402Client),
	)

	// Make request - payment is handled automatically
	// The PaymentRoundTripper will:
	// 1. Make the initial request
	// 2. If it receives a 402 Payment Required response, it will:
	//    - Parse the payment requirements from the response
	//    - Create a payment payload using the chain's DOMAIN_SEPARATOR
	//    - Sign the payment payload
	//    - Retry the request with the payment header
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		os.Exit(1)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := json.Marshal(map[string]interface{}{
			"status":  resp.StatusCode,
			"message": "Request failed",
		})
		fmt.Printf("❌ HTTP %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// Read response
	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("❌ Failed to decode response: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ Response received:")
	prettyJSON, _ := json.MarshalIndent(data, "  ", "  ")
	fmt.Printf("%s\n\n", string(prettyJSON))

	// Check payment response header
	paymentHeader := resp.Header.Get("PAYMENT-RESPONSE")
	if paymentHeader == "" {
		paymentHeader = resp.Header.Get("X-PAYMENT-RESPONSE")
	}

	if paymentHeader != "" {
		fmt.Println("💰 Payment settled successfully!")
		fmt.Printf("   Payment header: %s\n", paymentHeader)
	}
}
```

### 3. How It Works

The wrapped HTTP client automatically:

1. **Detects 402 responses**: When a server responds with `402 Payment Required`, the client extracts payment requirements from the `PAYMENT-REQUIRED` header.

2. **Creates payment payload**: The client uses registered payment schemes to create a signed payment payload.

3. **Retries with payment**: The client automatically retries the request with the `X-PAYMENT` header containing the payment payload.

4. **Handles settlement**: After successful payment verification, the server returns the resource and includes a `PAYMENT-RESPONSE` header with settlement confirmation.

---

## Configuration

### Environment Variables

For **Gate Web3 OpenAPI** authentication, set these environment variables:

```bash
# Required
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"

# Optional
export GATE_WEB3_PASSPHRASE="your-passphrase"
export GATE_WEB3_REAL_IP="your-real-ip"  # Defaults to 127.0.0.1
```

### Facilitator Configuration

The facilitator client uses Gate Web3 OpenAPI by default. It automatically uses Gate Web3 authentication if environment variables are set:

```go
facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
	URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	// Optional: Custom HTTP client
	HTTPClient: &http.Client{
		Timeout: 30 * time.Second,
	},
})
```

The client will automatically use the following environment variables for authentication:
- `GATE_WEB3_API_KEY`
- `GATE_WEB3_API_SECRET`
- `GATE_WEB3_PASSPHRASE` (optional)
- `GATE_WEB3_REAL_IP` (optional)

### Network Identifiers

x402 uses CAIP-2 format for network identifiers. Currently supported networks:

| Network | CAIP-2 Identifier | Default Asset |
|---------|-------------------|---------------|
| Gate Layer Testnet | `gatelayer_testnet` | USDC (`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`) |
| Gate Layer Mainnet | `gatelayer` | USDC (configured per network) |

**Payment Asset Selection:**

- When you specify a price in USD format (e.g., `"$0.001"`), the SDK automatically selects the default stablecoin for that network
- The default asset is configured in `go/mechanisms/evm/constants.go` for each network
- The SDK converts USD amounts to the token's smallest unit (e.g., $0.001 USDC = 1000 units for 6-decimal tokens)
- Clients automatically use the asset specified in the payment requirements from the server

**EIP-712 Domain Separator:**

- The SDK automatically uses the chain's DOMAIN_SEPARATOR for signing when available
- For `gatelayer_testnet` with USDC token (`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`), the SDK uses the chain's DOMAIN_SEPARATOR directly
- This ensures signatures match the token contract's EIP-712 domain configuration
- The SDK queries the DOMAIN_SEPARATOR from the chain when possible, or uses a pre-configured value for known tokens

---

## Next Steps

* Read the detailed [CLIENT.md](https://github.com/gatechain/x402/blob/main/go/CLIENT.md) documentation for building payment-enabled clients
* Read the detailed [SERVER.md](https://github.com/gatechain/x402/blob/main/go/SERVER.md) documentation for building payment-accepting servers
* Read the detailed [FACILITATOR.md](https://github.com/gatechain/x402/blob/main/go/FACILITATOR.md) documentation for building payment facilitators
* Explore the [examples](https://github.com/gatechain/x402/tree/main/examples/go) directory for more code samples

---

## Summary

### For Sellers

1. Install the x402 Go SDK
2. Create a facilitator client (Gate Web3 OpenAPI)
3. Configure payment routes
4. Add payment middleware to your server
5. Set environment variables for authentication (optional)

### For Buyers

1. Install the x402 Go SDK
2. Create a wallet signer from your private key
3. Create an x402 client and register payment schemes
4. Wrap your HTTP client with payment handling
5. Make requests - payments are handled automatically
   - The SDK automatically uses the chain's DOMAIN_SEPARATOR for signing
   - PaymentRoundTripper handles 402 responses and retries with payment automatically

### EIP-712 Signing

The SDK uses EIP-712 typed data signing for EIP-3009 `transferWithAuthorization`:

- **Automatic DOMAIN_SEPARATOR**: For known networks and tokens (like `gatelayer_testnet`), the SDK uses the chain's DOMAIN_SEPARATOR directly
- **Chain Compatibility**: Signatures are generated to match the token contract's EIP-712 domain configuration
- **No Manual Configuration**: You don't need to specify token name/version - the SDK handles it automatically

Your applications are now ready to accept and make crypto payments through x402!
