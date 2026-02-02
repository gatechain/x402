# x402 Go SDK 使用指南

本指南将引导您如何使用 **x402 Go SDK** 构建支持支付的应用程序。SDK 同时支持 **卖家**（接受支付的服务器）和 **买家**（发起付费请求的客户端）。

## 目录

- [卖家快速开始](#卖家快速开始)
- [买家快速开始](#买家快速开始)
- [配置](#配置)

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
go mod tidy
```

`go mod tidy` 命令会自动下载所有必需的依赖并更新您的 `go.mod` 和 `go.sum` 文件。

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

以下是使用 Gin 框架的完整可运行示例：

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

### 4. 测试您的集成

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

### 5. 支付资产选择

**资产如何选择：**

1. **服务器端（卖家）**：当您指定价格如 `"$0.001"` 时，SDK 会自动：
   - 解析 USD 金额
   - 查找指定网络的默认资产（在 `go/mechanisms/evm/constants.go` 中配置）
   - 对于 `gatelayer_testnet`，默认资产是地址为 `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` 的 USDC
   - 对于 `gatelayer`（主网），默认资产按网络配置
   - 将 USD 金额转换为代币的最小单位（例如，$0.001 = 1000，对于 6 位小数的 USDC）
   - SDK 使用链的 DOMAIN_SEPARATOR 进行签名，以确保与代币合约的兼容性

2. **客户端（买家）**：当客户端收到支付要求时：
   - 客户端过滤可用选项，仅保留与已注册方案/网络匹配的选项
   - 客户端使用支付要求中指定的资产地址创建支付

**各网络的默认资产：**

| Network | Default Asset | Address |
|---------|--------------|---------|
| `gatelayer_testnet` | USDC | `0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF` |
| `gatelayer` | USDC | (configured per network) |

**注意**：资产会根据网络配置自动确定。使用 USD 定价（`"$0.001"`）时，您无需指定资产地址。

**EIP-712 签名**：SDK 自动使用链的 DOMAIN_SEPARATOR 进行签名。对于 `gatelayer_testnet`，它使用代币合约的正确 DOMAIN_SEPARATOR 以确保签名有效。

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
go mod tidy
```

`go mod tidy` 命令会自动下载所有必需的依赖并更新您的 `go.mod` 和 `go.sum` 文件。

### 2. 创建支持支付的 HTTP 客户端

SDK 自动使用链的 DOMAIN_SEPARATOR 处理支付创建和签名。对于 `gatelayer_testnet`，它使用代币合约的正确 DOMAIN_SEPARATOR 以确保签名有效。

以下是完整可运行示例：

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

### 3. 工作原理

包装的 HTTP 客户端会自动：

1. **检测 402 响应**：当服务器响应 `402 Payment Required` 时，客户端从 `PAYMENT-REQUIRED` 头中提取支付要求。

2. **创建支付负载**：客户端使用已注册的支付方案创建签名的支付负载。

3. **使用支付重试**：客户端自动使用包含支付负载的 `X-PAYMENT` 头重试请求。

4. **处理结算**：支付验证成功后，服务器返回资源，并在 `PAYMENT-RESPONSE` 头中包含结算确认。

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

facilitator 客户端默认使用 Gate Web3 OpenAPI。如果设置了环境变量，它会自动使用 Gate Web3 认证：

```go
facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
	URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	// Optional: Custom HTTP client
	HTTPClient: &http.Client{
		Timeout: 30 * time.Second,
	},
})
```

客户端会自动使用以下环境变量进行认证：
- `GATE_WEB3_API_KEY`
- `GATE_WEB3_API_SECRET`
- `GATE_WEB3_PASSPHRASE` (可选)
- `GATE_WEB3_REAL_IP` (可选)

### 网络标识符

x402 使用 CAIP-2 格式作为网络标识符。当前支持的网络：

| Network | CAIP-2 Identifier | Default Asset |
|---------|-------------------|---------------|
| Gate Layer Testnet | `gatelayer_testnet` | USDC (`0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF`) |
| Gate Layer Mainnet | `gatelayer` | USDC (configured per network) |

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

## 下一步

* 阅读详细的 [CLIENT.md](https://github.com/gatechain/x402/blob/main/go/CLIENT.md) 文档以构建支持支付的客户端
* 阅读详细的 [SERVER.md](https://github.com/gatechain/x402/blob/main/go/SERVER.md) 文档以构建接受支付的服务器
* 阅读详细的 [FACILITATOR.md](https://github.com/gatechain/x402/blob/main/go/FACILITATOR.md) 文档以构建支付 facilitator
* 探索 [examples](https://github.com/gatechain/x402/tree/main/examples/go) 目录以获取更多代码示例

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
