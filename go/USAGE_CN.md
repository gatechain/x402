# x402 Go SDK 使用指南

本指南将引导您如何使用 **x402 Go SDK** 构建支持支付的应用程序。SDK 同时支持 **卖家**（接受支付的服务器）和 **买家**（发起付费请求的客户端）。

## 目录

- [卖家快速开始](#卖家快速开始)
- [买家快速开始](#买家快速开始)
- [配置](#配置)
- [示例](#示例)

---

## 卖家快速开始

本指南将向您展示如何将 x402 集成到您的 Go 服务器中，以接受 API 或服务的支付。

### 前置条件

在开始之前，请确保您拥有：

* 用于接收资金的加密钱包（任何兼容 EVM 的钱包）
* 已安装 [Go](https://go.dev/) 1.24+
* 现有的 HTTP 服务器（Gin、标准库等）

### 1. 安装依赖

将 x402 Go 模块添加到您的项目中：

```bash
go get github.com/gatechain/x402/go
```

### 2. 设置环境变量

在运行服务器之前，设置所需的环境变量：

```bash
# 必需：用于接收支付的钱包地址
export PAYEE_ADDRESS="0x1234567890123456789012345678901234567890"

# Gate Web3 OpenAPI 认证所需
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"

# 可选
export GATE_WEB3_PASSPHRASE="your-passphrase"
export GATE_WEB3_REAL_IP="your-real-ip"
```

### 3. 创建支付保护服务器

以下是使用 Gin 框架的完整示例：

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

### 4. 配置 Gate Web3 认证（可选）

如果设置了环境变量，facilitator 客户端会自动使用 Gate Web3 认证。在运行服务器之前设置这些变量：

```bash
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"
export GATE_WEB3_PASSPHRASE="your-passphrase"  # Optional
export GATE_WEB3_REAL_IP="your-real-ip"        # Optional, defaults to 127.0.0.1
```

如果未设置这些变量，您可以提供自定义的 `AuthProvider`：

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

### 5. 测试您的集成

1. 启动您的服务器：
   ```bash
   go run main.go
   ```

2. 发起不带支付的请求：
   ```bash
   curl http://localhost:4021/weather
   ```

3. 服务器会响应 `402 Payment Required`，并在 `PAYMENT-REQUIRED` 头中包含支付说明。

4. 使用兼容的客户端（参见[买家快速开始](#买家快速开始)）完成支付并重试请求。

5. 支付验证成功后，服务器会返回您的 API 响应。

### 6. 路由配置

路由定义了特定端点的支付要求：

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

#### 支付资产选择

**资产如何选择：**

1. **服务器端（卖家）**：当您指定价格如 `"$0.001"` 时，SDK 会自动：
   - 解析 USD 金额
   - 查找指定网络的默认资产（在 `mechanisms/evm/constants.go` 中配置）
   - 对于 `gatelayer_testnet`，默认资产是地址为 `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` 的 USDC
   - 将 USD 金额转换为代币的最小单位（例如，$0.001 = 1000，对于 6 位小数的 USDC）
   - SDK 使用链的 DOMAIN_SEPARATOR 进行签名，以确保与代币合约的兼容性

2. **客户端（买家）**：当客户端收到支付要求时：
   - 客户端过滤可用选项，仅保留与已注册方案/网络匹配的选项
   - 如果有多个选项可用，客户端选择第一个匹配的选项
   - 客户端使用支付要求中指定的资产地址创建支付

**各网络的默认资产：**

| Network | Default Asset | Address |
|---------|--------------|---------|
| `gatelayer_testnet` | USDC | `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` |
| `eip155:8453` (Base Mainnet) | USDC | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| `eip155:84532` (Base Sepolia) | USDC | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` |

**注意**：资产会根据网络配置自动确定。使用 USD 定价（`"$0.001"`）时，您无需指定资产地址。

**EIP-712 签名**：SDK 自动使用链的 DOMAIN_SEPARATOR 进行签名。对于 `gatelayer_testnet`，它使用代币合约的正确 DOMAIN_SEPARATOR 以确保签名有效。

### 7. 多网络支持

您可以在同一端点支持多个网络：

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

## 买家快速开始

本指南将向您展示如何创建一个 Go 客户端，可以向 x402 保护的资源发起付费请求。

### 前置条件

在开始之前，请确保您拥有：

* 拥有 USDC 的加密钱包（任何兼容 EVM 的钱包）
* 已安装 [Go](https://go.dev/) 1.24+
* 需要通过 x402 支付的服务

### 1. 安装依赖

将 x402 Go 模块添加到您的项目中：

```bash
go get github.com/gatechain/x402/go
```

### 2. 创建钱包签名器

从您的私钥创建签名器：

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

### 4. 创建支持支付的 HTTP 客户端

SDK 自动使用链的 DOMAIN_SEPARATOR 处理支付创建和签名。对于 `gatelayer_testnet`，它使用代币合约的正确 DOMAIN_SEPARATOR 以确保签名有效。

以下是完整示例：

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

### 5. 工作原理

包装的 HTTP 客户端会自动：

1. **检测 402 响应**：当服务器响应 `402 Payment Required` 时，客户端从 `PAYMENT-REQUIRED` 头中提取支付要求。

2. **创建支付负载**：客户端使用已注册的支付方案创建签名的支付负载。

3. **使用支付重试**：客户端自动使用包含支付负载的 `X-PAYMENT` 头重试请求。

4. **处理结算**：支付验证成功后，服务器返回资源，并在 `PAYMENT-RESPONSE` 头中包含结算确认。

### 6. 多网络客户端设置

您可以注册多个支付方案以处理不同的网络：

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

### 7. 错误处理

客户端在以下情况下会返回错误：

* 没有为所需网络注册方案
* 支付负载创建失败
* 支付验证失败
* 请求超时

错误处理示例：

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

## 配置

### 环境变量

对于 **Gate Web3 OpenAPI** 认证，设置以下环境变量：

```bash
# 必需
export GATE_WEB3_API_KEY="your-api-key"
export GATE_WEB3_API_SECRET="your-api-secret"

# 可选
export GATE_WEB3_PASSPHRASE="your-passphrase"
export GATE_WEB3_REAL_IP="your-real-ip"  # Defaults to 127.0.0.1
```

### Facilitator 配置

facilitator 客户端默认使用 Gate Web3 OpenAPI：

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

### 网络标识符

x402 使用 CAIP-2 格式作为网络标识符：

| Network | CAIP-2 Identifier | Default Asset |
|---------|-------------------|---------------|
| Gate Layer Testnet | `gatelayer_testnet` | USDC (`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`) |
| Gate Layer Mainnet | `gatelayer` | USDC (configured per network) |
| Base Mainnet | `eip155:8453` | USDC (`0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913`) |
| Base Sepolia | `eip155:84532` | USDC (`0x036CbD53842c5426634e7929541eC2318f3dCF7e`) |

**支付资产选择：**

- 当您以 USD 格式指定价格（例如 `"$0.001"`）时，SDK 会自动为该网络选择默认稳定币
- 默认资产在 `go/mechanisms/evm/constants.go` 中为每个网络配置
- SDK 将 USD 金额转换为代币的最小单位（例如，$0.001 USDC = 1000 单位，对于 6 位小数的代币）
- 客户端自动使用服务器支付要求中指定的资产

**EIP-712 域分隔符：**

- SDK 在可用时自动使用链的 DOMAIN_SEPARATOR 进行签名
- 对于 `gatelayer_testnet` 上的 USDC 代币（`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`），SDK 直接使用链的 DOMAIN_SEPARATOR
- 这确保签名与代币合约的 EIP-712 域配置匹配
- SDK 在可能时从链查询 DOMAIN_SEPARATOR，或对已知代币使用预配置值

---

## 示例

### 完整服务器示例

查看 [`examples/go/servers/gin/`](../examples/go/servers/gin/) 获取完整的 Gin 服务器示例。

### 完整客户端示例

查看 [`examples/go/clients/http/`](../examples/go/clients/http/) 获取完整的 HTTP 客户端示例。

### 高级示例

* **自定义传输**： [`examples/go/clients/advanced/`](../examples/go/clients/advanced/)
* **动态定价**： [`examples/go/servers/advanced/`](../examples/go/servers/advanced/)
* **Bazaar 发现**： [`examples/go/servers/advanced/bazaar.go`](../examples/go/servers/advanced/bazaar.go)

---

## 下一步

* 阅读详细的 [CLIENT.md](CLIENT.md) 文档以构建支持支付的客户端
* 阅读详细的 [SERVER.md](SERVER.md) 文档以构建接受支付的服务器
* 阅读详细的 [FACILITATOR.md](FACILITATOR.md) 文档以构建支付 facilitator
* 探索 [examples](../examples/go/) 目录以获取更多代码示例

---

## 总结

### 对于卖家

1. 安装 x402 Go SDK
2. 创建 facilitator 客户端（Gate Web3 OpenAPI）
3. 配置支付路由
4. 向服务器添加支付中间件
5. 设置环境变量以进行认证（可选）

### 对于买家

1. 安装 x402 Go SDK
2. 从您的私钥创建钱包签名器
3. 创建 x402 客户端并注册支付方案
4. 使用支付处理包装您的 HTTP 客户端
5. 发起请求 - 支付会自动处理
   - SDK 自动使用链的 DOMAIN_SEPARATOR 进行签名
   - PaymentRoundTripper 自动处理 402 响应并使用支付重试

### EIP-712 签名

SDK 使用 EIP-712 类型化数据签名来实现 EIP-3009 `transferWithAuthorization`：

- **自动 DOMAIN_SEPARATOR**：对于已知网络和代币（如 `gatelayer_testnet`），SDK 直接使用链的 DOMAIN_SEPARATOR
- **链兼容性**：生成的签名与代币合约的 EIP-712 域配置匹配
- **无需手动配置**：您无需指定代币名称/版本 - SDK 会自动处理

您的应用程序现在已准备好通过 x402 接受和发起加密支付！
