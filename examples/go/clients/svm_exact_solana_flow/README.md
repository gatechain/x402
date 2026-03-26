# Go demo: SVM（Solana）exact 付款客户端

这个 demo 是一个 **x402 客户端**：先请求 `PAYMENT-REQUIRED`，再构造 Solana 交易并在请求头带 `PAYMENT-SIGNATURE` 重试。

## 前置

- 先启动资源服务器（例如本仓库的 `examples/go/servers/svm_exact_solana_flow`）
- 准备一把 Solana devnet 钱包私钥（base58）并确保有测试币/对应代币余额

## 运行

```bash
go run .
```

默认请求 `http://localhost:4024/pay`。

## 环境变量

- **SERVER_URL**: 默认 `http://localhost:4024/pay`
- **SVM_NETWORK**: 默认 `solana-devnet`（也支持 CAIP-2：`solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`）
- **SVM_CLIENT_PRIVATE_KEY**: **必填**，Solana 私钥（base58）

## 你需要提供的代币信息（后续我来补齐）

你给我以下信息后，我会把 server/client 的默认配置、校验、以及 README 里的“如何换币/换网络”写完整：

- 目标网络（mainnet/devnet/testnet 的 CAIP-2）
- 代币 mint 地址
- decimals
- 建议的默认 `PAYMENT_AMOUNT_ATOMIC`

