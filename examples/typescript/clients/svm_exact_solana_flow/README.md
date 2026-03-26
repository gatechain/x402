# TS demo（一体化）: SVM（Solana）exact 付款流（openapi-test）

这个 demo 把 **server（资源方）** 和 **client（付款方）** 放在同一个进程里：

- 本地 Express + `@x402/express` 中间件保护 `GET /pay`
- 同进程 client：先拿 `PAYMENT-REQUIRED`，再带 `PAYMENT-SIGNATURE` 重试

> 注意：Gate openapi-test 的 `/supported` 目前不返回 `feePayer/signers`，因此需要你显式提供 `SVM_FEE_PAYER`（base58 地址，不是私钥）。

## 运行

```bash
pnpm start
```

默认 URL：`http://localhost:4025/pay`

## 环境变量

- **GATE_WEB3_API_KEY / GATE_WEB3_API_SECRET**: 必填（用于调用 openapi-test facilitator）
- **FACILITATOR_URL**: 默认 `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: 默认 `solana-devnet`（Gate openapi-test verify 只接受 V1 网络名）
- **SVM_ASSET_MINT**: 默认 `BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`
- **PAYMENT_AMOUNT_ATOMIC**: 默认 `100000`（6 位精度即 0.10）
- **SVM_PAYEE_ADDRESS**: 必填，收款地址（base58）
- **SVM_FEE_PAYER**: 必填，fee payer 地址（base58）
- **SVM_CLIENT_PRIVATE_KEY**: 必填，付款方私钥（base58 bytes）
- **PORT**: 默认 `4025`
- **ROUTE_PATH**: 默认 `/pay`
- **KEEP_ALIVE**: 设为 `1` 则跑完 client 后不关闭 server

