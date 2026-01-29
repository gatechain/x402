# x402 Go SDK Usage Guide

This guide walks you through how to use the **x402 Go SDK** to build payment-enabled applications. The SDK supports both **sellers** (servers that accept payments) and **buyers** (clients that make paid requests).

## Table of Contents

- [Quickstart for Sellers](#quickstart-for-sellers)
- [Quickstart for Buyers](#quickstart-for-buyers)
- [Configuration](#configuration)
- [Examples](#examples)

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
```

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

Here's a complete example using Gin framework:

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

### 4. Configure Gate Web3 Authentication (Optional)

The facilitator client automatically uses Gate Web3 authentication if environment variables are set. Set these before running your server:

```bash
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"
export GATE_WEB3_PASSPHRASE="your-passphrase"  # Optional
export GATE_WEB3_REAL_IP="your-real-ip"        # Optional, defaults to 127.0.0.1
```

If these are not set, you can provide a custom `AuthProvider`:

```go
package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	x402http "github.com/gatechain/x402/go/http"
)

// MyAuthProvider implements custom authentication for Gate Web3 OpenAPI
type MyAuthProvider struct {
	apiKey     string
	apiSecret  string
	passphrase string
	realIP     string
}

func NewMyAuthProvider() *MyAuthProvider {
	return &MyAuthProvider{
		apiKey:     os.Getenv("GATE_WEB3_API_KEY"),
		apiSecret:  os.Getenv("GATE_WEB3_API_SECRET"),
		passphrase: os.Getenv("GATE_WEB3_PASSPHRASE"),
		realIP:     os.Getenv("GATE_WEB3_REAL_IP"),
	}
}

func (a *MyAuthProvider) GetAuthHeaders(ctx context.Context) (x402http.AuthHeaders, error) {
	timestamp := time.Now().UnixMilli()
	requestID := uuid.NewString()

	// Create signature for verify endpoint
	verifyHeaders := a.createHeaders(timestamp, requestID, "v1/x402/verify")
	settleHeaders := a.createHeaders(timestamp, requestID, "v1/x402/settle")
	supportedHeaders := a.createHeaders(timestamp, requestID, "v1/x402/supported")

	return x402http.AuthHeaders{
		Verify:    verifyHeaders,
		Settle:    settleHeaders,
		Supported: supportedHeaders,
	}, nil
}

func (a *MyAuthProvider) createHeaders(timestamp int64, requestID, targetURI string) map[string]string {
	headers := map[string]string{
		"X-Api-Key":     a.apiKey,
		"X-Timestamp":   strconv.FormatInt(timestamp, 10),
		"X-Request-Id":  requestID,
		"x-target-uri":  targetURI,
	}

	if a.passphrase != "" {
		headers["X-Passphrase"] = a.passphrase
	}

	if a.realIP != "" {
		headers["X-Forwarded-For"] = a.realIP
	}

	// Note: X-Signature is calculated by the SDK automatically
	// If you need custom signature logic, implement it here

	return headers
}

// Usage:
facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
	URL:          "https://openapi-test.gateweb3.cc/api/v1/x402",
	AuthProvider: NewMyAuthProvider(),
})
```

### 5. Test Your Integration

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

### 6. Route Configuration

Routes define payment requirements for specific endpoints:

```go
package main

import (
	"os"

	x402http "github.com/gatechain/x402/go/http"
)

func main() {
	// Get payee address from environment
	payeeAddress := os.Getenv("PAYEE_ADDRESS")
	if payeeAddress == "" {
		payeeAddress = "0x1234567890123456789012345678901234567890" // Example address
	}

	routes := x402http.RoutesConfig{
		"GET /weather": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",           // Payment scheme
					PayTo:   payeeAddress,      // Payment recipient address
					Price:   "$0.001",         // Price in USD
					Network: "gatelayer_testnet", // Network identifier
				},
			},
			Description: "Get weather data for a city",
			MimeType:    "application/json",
		},
		"POST /api/data": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",
					PayTo:   payeeAddress,
					Price:   "$0.01",
					Network: "gatelayer_testnet",
				},
			},
			Description: "Submit data to the API",
			MimeType:    "application/json",
		},
		"GET /api/premium": {
			Accepts: x402http.PaymentOptions{
				{
					Scheme:  "exact",
					PayTo:   payeeAddress,
					Price:   "$0.10",
					Network: "gatelayer_testnet",
				},
			},
			Description: "Access premium content",
			MimeType:    "application/json",
		},
	}

	_ = routes // Use routes in middleware configuration
}
```

#### Payment Asset Selection

**How assets are selected:**

1. **Server-side (Seller)**: When you specify a price like `"$0.001"`, the SDK automatically:
   - Parses the USD amount
   - Looks up the default asset for the specified network (configured in `mechanisms/evm/constants.go`)
   - For `gatelayer_testnet`, the default asset is USDC at address `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`
   - Converts the USD amount to the token's smallest unit (e.g., $0.001 = 1000 for USDC with 6 decimals)
   - The SDK uses the chain's DOMAIN_SEPARATOR for signing to ensure compatibility with the token contract

2. **Client-side (Buyer)**: When the client receives payment requirements:
   - The client filters available options to those matching registered schemes/networks
   - If multiple options are available, the client selects the first matching one
   - The client uses the asset address specified in the payment requirements to create the payment

**Default Assets by Network:**

| Network | Default Asset | Address |
|---------|--------------|---------|
| `gatelayer_testnet` | USDC | `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` |
| `eip155:8453` (Base Mainnet) | USDC | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| `eip155:84532` (Base Sepolia) | USDC | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` |

**Note**: The asset is automatically determined from the network configuration. You don't need to specify the asset address when using USD pricing (`"$0.001"`).

**EIP-712 Signing**: The SDK automatically uses the chain's DOMAIN_SEPARATOR for signing. For `gatelayer_testnet`, it uses the correct DOMAIN_SEPARATOR from the token contract to ensure signatures are valid.

### 7. Multi-Network Support

You can support multiple networks on the same endpoint:

```go
package main

import (
	"os"
	"time"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evm "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
	svm "github.com/gatechain/x402/go/mechanisms/svm/exact/server"
	"github.com/gin-gonic/gin"
)

func main() {
	evmPayee := os.Getenv("EVM_PAYEE_ADDRESS")
	svmPayee := os.Getenv("SVM_PAYEE_ADDRESS")

	r := gin.Default()
	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	})

	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes: x402http.RoutesConfig{
			"GET /weather": {
				Accepts: x402http.PaymentOptions{
					{
						Scheme:  "exact",
						PayTo:   evmPayee,
						Price:   "$0.001",
						Network: "gatelayer_testnet",
					},
					{
						Scheme:  "exact",
						PayTo:   svmPayee,
						Price:   "$0.001",
						Network: "solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1",
					},
				},
				Description: "Get weather data",
				MimeType:    "application/json",
			},
		},
		Facilitator: facilitatorClient,
		Schemes: []ginmw.SchemeConfig{
			{Network: x402.Network("gatelayer_testnet"), Server: evm.NewExactEvmScheme()},
			{Network: x402.Network("solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1"), Server: svm.NewExactSvmScheme()},
		},
		Timeout: 30 * time.Second,
	}))

	r.Run(":4021")
}
```

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
```

### 2. Create a Wallet Signer

Create a signer from your private key:

```go
package main

import (
	"fmt"
	"log"
	"os"

	evmsigners "github.com/gatechain/x402/go/signers/evm"
)

func main() {
	// Load private key from environment variable
	privateKey := os.Getenv("EVM_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatal("❌ EVM_PRIVATE_KEY environment variable is required")
	}

	// Create EVM signer
	evmSigner, err := evmsigners.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		log.Fatalf("❌ Failed to create signer: %v", err)
	}

	fmt.Printf("✅ Signer created successfully\n")
	fmt.Printf("   Address: %s\n", evmSigner.Address())
}
```

### 4. Create a Payment-Enabled HTTP Client

The SDK automatically handles payment creation and signing using the chain's DOMAIN_SEPARATOR. For `gatelayer_testnet`, it uses the correct DOMAIN_SEPARATOR from the token contract to ensure signatures are valid.

Here's a complete example:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
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

	// Create EVM signer
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

### 5. How It Works

The wrapped HTTP client automatically:

1. **Detects 402 responses**: When a server responds with `402 Payment Required`, the client extracts payment requirements from the `PAYMENT-REQUIRED` header.

2. **Creates payment payload**: The client uses registered payment schemes to create a signed payment payload.

3. **Retries with payment**: The client automatically retries the request with the `X-PAYMENT` header containing the payment payload.

4. **Handles settlement**: After successful payment verification, the server returns the resource and includes a `PAYMENT-RESPONSE` header with settlement confirmation.

### 6. Multi-Network Client Setup

You can register multiple payment schemes to handle different networks:

```go
package main

import (
	"fmt"
	"net/http"
	"os"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	evm "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	svm "github.com/gatechain/x402/go/mechanisms/svm/exact/client"
	evmsigners "github.com/gatechain/x402/go/signers/evm"
	svmsigners "github.com/gatechain/x402/go/signers/svm"
)

func main() {
	// Create signers
	evmPrivateKey := os.Getenv("EVM_PRIVATE_KEY")
	svmPrivateKey := os.Getenv("SVM_PRIVATE_KEY")

	if evmPrivateKey == "" && svmPrivateKey == "" {
		fmt.Println("❌ At least one of EVM_PRIVATE_KEY or SVM_PRIVATE_KEY is required")
		os.Exit(1)
	}

	var evmSigner evmsigners.ClientEvmSigner
	var svmSigner svmsigners.ClientSvmSigner
	var err error

	if evmPrivateKey != "" {
		evmSigner, err = evmsigners.NewClientSignerFromPrivateKey(evmPrivateKey)
		if err != nil {
			fmt.Printf("❌ Failed to create EVM signer: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ EVM signer created: %s\n", evmSigner.Address())
	}

	if svmPrivateKey != "" {
		svmSigner, err = svmsigners.NewClientSignerFromPrivateKey(svmPrivateKey)
		if err != nil {
			fmt.Printf("❌ Failed to create SVM signer: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ SVM signer created: %s\n", svmSigner.Address())
	}

	// Create client with multiple schemes
	x402Client := x402.Newx402Client()

	if evmSigner != nil {
		x402Client = x402Client.Register("gatelayer_testnet", evm.NewExactEvmScheme(evmSigner))
	}

	if svmSigner != nil {
		x402Client = x402Client.Register("solana:*", svm.NewExactSvmScheme(svmSigner))
	}

	// Wrap HTTP client with payment handling
	httpClient := x402http.WrapHTTPClientWithPayment(
		http.DefaultClient,
		x402http.Newx402HTTPClient(x402Client),
	)

	// Now handles both EVM and Solana networks automatically!
	fmt.Println("✅ Multi-network client ready")
	_ = httpClient // Use httpClient for requests
}
```

### 7. Error Handling

The client will return errors if:

* No scheme is registered for the required network
* The payment payload creation fails
* The payment verification fails
* The request times out

Example error handling:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	x402 "github.com/gatechain/x402/go"
)

func handleRequestError(err error) {
	if err == nil {
		return
	}

	// Check for specific error types
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "No scheme registered"):
		fmt.Println("❌ Network not supported - register the appropriate scheme")
		fmt.Println("   Example: client.Register(\"gatelayer_testnet\", evmScheme)")
	case strings.Contains(errMsg, "Payment verification failed"):
		fmt.Println("❌ Payment was rejected by the facilitator")
		fmt.Println("   Check your wallet balance and payment requirements")
	case strings.Contains(errMsg, "402 Payment Required"):
		fmt.Println("❌ Payment required but failed to create payment payload")
		fmt.Println("   Check your signer configuration")
	case strings.Contains(errMsg, "context deadline exceeded"):
		fmt.Println("❌ Request timeout - the server took too long to respond")
	default:
		fmt.Printf("❌ Request failed: %v\n", err)
	}

	// Try to extract more details from error
	var verifyErr *x402.VerifyError
	if errors.As(err, &verifyErr) {
		fmt.Printf("   Reason: %s\n", verifyErr.Reason)
		fmt.Printf("   Payer: %s\n", verifyErr.Payer)
		fmt.Printf("   Network: %s\n", verifyErr.Network)
	}

	var settleErr *x402.SettleError
	if errors.As(err, &settleErr) {
		fmt.Printf("   Reason: %s\n", settleErr.Reason)
		fmt.Printf("   Transaction: %s\n", settleErr.Transaction)
		fmt.Printf("   Network: %s\n", settleErr.Network)
	}

	os.Exit(1)
}

// Usage in main:
resp, err := httpClient.Do(req)
if err != nil {
	handleRequestError(err)
}
```

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

The facilitator client uses Gate Web3 OpenAPI by default:

```go
facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
	URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	// Optional: Custom HTTP client
	HTTPClient: &http.Client{
		Timeout: 30 * time.Second,
	},
	// Optional: Custom auth provider
	AuthProvider: &MyAuthProvider{},
})
```

### Network Identifiers

x402 uses CAIP-2 format for network identifiers:

| Network | CAIP-2 Identifier | Default Asset |
|---------|-------------------|---------------|
| Gate Layer Testnet | `gatelayer_testnet` | USDC (`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`) |
| Gate Layer Mainnet | `gatelayer` | USDC (configured per network) |
| Base Mainnet | `eip155:8453` | USDC (`0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913`) |
| Base Sepolia | `eip155:84532` | USDC (`0x036CbD53842c5426634e7929541eC2318f3dCF7e`) |

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

## Examples

### Complete Server Example

See [`examples/go/servers/gin/`](../examples/go/servers/gin/) for a complete Gin server example.

### Complete Client Example

See [`examples/go/clients/http/`](../examples/go/clients/http/) for a complete HTTP client example.

### Advanced Examples

* **Custom Transport**: [`examples/go/clients/advanced/`](../examples/go/clients/advanced/)
* **Dynamic Pricing**: [`examples/go/servers/advanced/`](../examples/go/servers/advanced/)
* **Bazaar Discovery**: [`examples/go/servers/advanced/bazaar.go`](../examples/go/servers/advanced/bazaar.go)

---

## Next Steps

* Read the detailed [CLIENT.md](CLIENT.md) documentation for building payment-enabled clients
* Read the detailed [SERVER.md](SERVER.md) documentation for building payment-accepting servers
* Read the detailed [FACILITATOR.md](FACILITATOR.md) documentation for building payment facilitators
* Explore the [examples](../examples/go/) directory for more code samples

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
