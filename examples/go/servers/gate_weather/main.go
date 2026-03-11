package main

import (
	"fmt"
	nethttp "net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evm "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
)

// Simple weather demo server on Gate Layer testnet using x402 V2 + Gate Web3 facilitator.
//
// 环境变量：
// - PAYEE_ADDRESS        收款地址（EVM 地址）
// - GATE_WEB3_API_KEY    Gate Web3 AK
// - GATE_WEB3_API_SECRET Gate Web3 SK
// - GATE_WEB3_PASSPHRASE (可选)
// - GATE_WEB3_REAL_IP    (可选)
//
// 运行示例（请替换为你自己的值）：
//
//   export PAYEE_ADDRESS=0xYourReceiverAddress
//   export GATE_WEB3_API_KEY=AMVVFAHZZ6OSJOKJ7SYRFYA2DQ
//   export GATE_WEB3_API_SECRET='HnTgZZy0lzOt1wRecWoJOo7Baf1CTDRrwwVjK5yOlDM.cLhwSs1u'
//
//   cd go
//   go run ../examples/go/servers/gate_weather
//
// 然后访问（无支付会返回 402）：
//   curl -i http://localhost:4021/weather
//
func main() {
	// 收款地址从环境变量读取
	payTo := os.Getenv("PAYEE_ADDRESS")
	if payTo == "" {
		fmt.Println("❌ PAYEE_ADDRESS environment variable is required")
		fmt.Println("   Example: export PAYEE_ADDRESS=0x1234567890123456789012345678901234567890")
		os.Exit(1)
	}

	network := x402.Network("gatelayer_testnet") // Gate Layer testnet（在 EVM 配置里已绑定默认 USDC）

	fmt.Printf("🚀 Starting x402 Gate weather server...\n")
	fmt.Printf("   Payee address: %s\n", payTo)
	fmt.Printf("   Network: %s\n", network)
	fmt.Printf("   Facilitator: https://openapi-test.gateweb3.cc/api/v1/x402\n\n")

	r := gin.Default()

	// Gate Web3 OpenAPI Testnet facilitator client
	// 会自动读取 GATE_WEB3_API_KEY / GATE_WEB3_API_SECRET 等环境变量做签名
	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: "https://openapi-test.gateweb3.cc/api/v1/x402",
	})

	// 配置 x402 支付中间件，只保护 GET /weather 路由
	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes: x402http.RoutesConfig{
			"GET /weather": {
				Accepts: x402http.PaymentOptions{
					{
						Scheme:  "exact",
						PayTo:   payTo,
						Price:   "$0.001", // 以美元计价，会自动换算为该网络默认 USDC 的最小单位
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

	// 受保护的 weather 接口
	r.GET("/weather", func(c *gin.Context) {
		city := c.DefaultQuery("city", "San Francisco")
		c.JSON(nethttp.StatusOK, gin.H{
			"city":        city,
			"weather":     "sunny",
			"temperature": 70,
			"timestamp":   time.Now().Format(time.RFC3339),
		})
	})

	// 不需要支付的健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(nethttp.StatusOK, gin.H{
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

