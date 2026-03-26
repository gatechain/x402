# Go demo: x402 exact on Solana (SVM)

这个 demo 是一个 **x402 资源服务器**（Gin 中间件），在 Solana 网络上使用 **`exact`** 方案收款。

它会：

- 对 `GET /pay` 返回 402，并在响应头 `PAYMENT-REQUIRED` 里给出 `accepts[]`
- 客户端带 `PAYMENT-SIGNATURE` 重试后，通过 facilitator 验证并结算后放行

## 运行

```bash
go run .
```

默认监听 `http://localhost:4024/pay`。

## 环境变量

- **FACILITATOR_URL**: 默认 `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: 默认 `solana-devnet`（也支持 CAIP-2：`solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`）
- **SVM_PAYEE_ADDRESS**: **必填**，Solana 收款地址（base58）
- **SVM_ASSET_MINT**: 代币 mint（base58），默认 openapi-test devnet 资产（`BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`）
- **PAYMENT_AMOUNT_ATOMIC**: 原子金额（最小单位字符串），默认 `100000`（若 6 decimals 即 0.10）
- **PORT**: 默认 `4024`
- **ROUTE_PATH**: 默认 `/pay`

> 你后续给我“代币信息”（mint/decimals/默认金额/网络）后，我会把默认值和校验补齐，并在 README 里写清楚换币步骤。

