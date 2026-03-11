package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	gin "github.com/gin-gonic/gin"

	x402 "github.com/gatechain/x402/go"
	x402http "github.com/gatechain/x402/go/http"
	ginmw "github.com/gatechain/x402/go/http/gin"
	evmserver "github.com/gatechain/x402/go/mechanisms/evm/exact/server"
	evmclient "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/gatechain/x402/go/signers/evm"
)

// TestGateWeatherEndToEnd 使用真实 Gate facilitator：
// - 启动一个接入 Gate OpenAPI 的本地 server（/weather 路由）
// - 用集成了我们 SDK 的客户端（有私钥）访问该路由，自动完成支付
func TestGateWeatherEndToEnd(t *testing.T) {
	t.Helper()

	payTo := os.Getenv("PAYEE_ADDRESS")
	evmPrivKey := os.Getenv("EVM_PRIVATE_KEY")
	ak := os.Getenv("GATE_WEB3_API_KEY")
	sk := os.Getenv("GATE_WEB3_API_SECRET")

	if payTo == "" || evmPrivKey == "" || ak == "" || sk == "" {
		t.Skip("PAYEE_ADDRESS, EVM_PRIVATE_KEY, GATE_WEB3_API_KEY, GATE_WEB3_API_SECRET 必须设置才跑这个真实环境集成测试")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()

	network := x402.Network("gatelayer_testnet")

	// 使用真实 Gate Web3 OpenAPI facilitator 客户端
	facilitatorClient := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{
		URL: x402http.DefaultFacilitatorURL,
	})

	// 接入 x402 支付中间件，保护 /weather
	r.Use(ginmw.X402Payment(ginmw.Config{
		Routes: x402http.RoutesConfig{
			"GET /weather": {
				Accepts: x402http.PaymentOptions{
					{
						Scheme:  "exact",
						PayTo:   payTo,
						Price:   "$0.001", // USD 价格，由 SDK 自动换算为该网络默认 USDC
						Network: network,
					},
				},
				Description: "Gate weather demo",
				MimeType:    "application/json",
			},
		},
		Facilitator:           facilitatorClient,
		Schemes:               []ginmw.SchemeConfig{{Network: network, Server: evmserver.NewExactEvmScheme()}},
		SyncFacilitatorOnStart: true,
		Timeout:                30 * time.Second,
	}))

	r.GET("/weather", func(c *gin.Context) {
		city := c.DefaultQuery("city", "Shanghai")
		c.JSON(http.StatusOK, gin.H{
			"city":        city,
			"weather":     "sunny",
			"temperature": 26,
			"timestamp":   time.Now().Format(time.RFC3339),
		})
	})

	server := httptest.NewServer(r)
	defer server.Close()

	// ---------- 客户端：使用我们的 SDK + 私钥 ----------
	signer, err := evmsigners.NewClientSignerFromPrivateKey(evmPrivKey)
	if err != nil {
		t.Fatalf("NewClientSignerFromPrivateKey: %v", err)
	}

	clientCore := x402.Newx402Client()
	// 注册 gatelayer_testnet 网络的 exact 机制
	clientCore.Register("gatelayer_testnet", evmclient.NewExactEvmScheme(signer))

	httpClient := x402http.WrapHTTPClientWithPayment(nil, x402http.Newx402HTTPClient(clientCore))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/weather", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["city"] == "" {
		t.Fatalf("unexpected response body: %#v", body)
	}
}

