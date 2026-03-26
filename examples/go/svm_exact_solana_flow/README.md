# Go demo（一体化）: SVM（Solana）exact 付款流

这个 demo 把 **server（资源方）** 和 **client（付款方）** 整合到同一个 `main.go`：

- 启动本地 Gin + x402 中间件：`GET /pay`
- 同进程跑 client：先拿 `PAYMENT-REQUIRED`，再带 `PAYMENT-SIGNATURE` 重试
- 默认跑完自动退出；如需一直跑 server，设置 `KEEP_ALIVE=1`

## 运行

```bash
go run .
```

默认地址：`http://localhost:4024/pay`

## 环境变量

- **FACILITATOR_URL**: 默认 `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: 默认 `solana-devnet`。也可以填 CAIP-2（`solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`），但 **Gate openapi-test verify 目前只接受 V1 网络名**，本 demo 会自动把 CAIP-2 转回 V1 再请求 facilitator。
- **SVM_PAYEE_ADDRESS**: **必填**，Solana 收款地址（base58）
- **SVM_ASSET_MINT**: 默认 openapi-test devnet 资产（`BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`）
- **PAYMENT_AMOUNT_ATOMIC**: 默认 `100000`（6 位精度即 0.10）
- **SVM_FEE_PAYER**: Solana fee payer（base58）。**Gate openapi-test 的 `/supported` 目前不返回 feePayer/signers**，所以要跑通 SVM exact demo，需要你显式提供一个 fee payer 地址（通常是 facilitator 管理的地址）。
- **SVM_CLIENT_PRIVATE_KEY**: **必填**，Solana 私钥（base58）
- **PORT**: 默认 `4024`
- **ROUTE_PATH**: 默认 `/pay`
- **KEEP_ALIVE**: 设为 `1` 则跑完 client 后不退出

