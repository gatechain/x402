# 新增 SDK 用法汇总（Permit2 + Solana SVM）

本文档汇总本次新增/改造的 4 个示例：

1. Go - Permit2(BSC) 客户端
2. TypeScript - Permit2(BSC) 一体化（server+client）
3. Go - Solana SVM 一体化（server+client）
4. TypeScript - Solana SVM 一体化（server+client）

> 安全提醒：私钥只放本地环境变量或 `.env`，不要提交到仓库，也不要发到聊天记录。

---

## 第一步：扩充支持代币（网络 / Token / 地址）

按你的要求，这里先只维护三列：**网络、Token、CA 地址（EVM 为合约地址；Solana 为 Mint）**。  
另外把 **非 EIP-3009** 单独拆表。

### 表 A：EIP-3009 代币（可走 EIP-3009 路径）

| 网络 | Token | CA 地址 |
|---|---|---|
| ETH | USDT | `0xdac17f958d2ee523a2206206994597c13d831ec7` |
| BSC | USDC | `0x8ac76a51cc950d9822d68b83fe1ad97b32cd580d` |
| BSC | USDT | `0x55d398326f99059ff775485246999027b3197955` |
| BSC | USD1 | `0x8d0d000ee44948fc98c9b98a4fa4921476f08b0d` |
| BASE | USDT | `0xfde4c96c8593536e31f229ea8f37b2ada2699bb2` |

### 表 B：非 EIP-3009（含非 EVM 链）

| 网络 | Token | CA 地址 |
|---|---|---|
| ETH | USDC | `0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48` |
| BASE | USDC | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| Arbitrum One | USDC | `0xaf88d065e77c8cc2239327c5edb3a432268e5831` |
| Polygon | USDC | `0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359` |
| Gatelayer | GUSD | `0xECE3F96198a5E6B9b2278edbEa8d548F66050d1c` |
| Gatelayer | usdc.e | `0x8a2B28364102Bea189D99A475C494330Ef2bDD0B` |
| Solana | USDC | `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` |
| Solana | USDT | `Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB` |

> 说明：Solana 为非 EVM 链，天然不走 EIP-3009。

---

## 通用前置

- 使用 Gate openapi-test facilitator 时，需配置：
  - `GATE_WEB3_API_KEY`
  - `GATE_WEB3_API_SECRET`
  - 可选：`GATE_WEB3_PASSPHRASE`
  - 可选：`GATE_WEB3_REAL_IP`
  - 可选：`FACILITATOR_URL`（默认 `https://openapi-test.gateweb3.cc/api/v1/x402`）

---

## 1) Go Permit2(BSC) 客户端

- 路径：`examples/go/clients/permit2_exact_bsc_flow`

### 必需环境变量

- `EVM_PRIVATE_KEY`：付款方 EVM 私钥（`0x...`）

### 常用可选

- `SERVER_URL`：默认 `http://localhost:4023/pay`
- `BSC_NETWORK`：默认 `eip155:56`

### 运行

```bash
cd examples/go/clients/permit2_exact_bsc_flow
go run .
```

---

## 2) TypeScript Permit2(BSC) 一体化

- 路径：`examples/typescript/clients/permit2_exact_bsc_flow`

### 必需环境变量

- `EVM_PRIVATE_KEY`
- `EVM_PAYEE_ADDRESS`
- `GATE_WEB3_API_KEY`
- `GATE_WEB3_API_SECRET`

### 常用可选

- `FACILITATOR_URL`
- `PERMIT_SPENDER`
- `PERMIT_NONCE`（不填时默认每次运行随机，避免 `nonce already used`）
- `PERMIT_DEADLINE`
- `WITNESS_VALID_AFTER`
- `PAYMENT_AMOUNT`

### 运行

```bash
# 首次或变更依赖后
cd examples/typescript
pnpm install --no-frozen-lockfile

# 如改过 packages/core 等源码，先构建
cd ../../typescript
pnpm build

# 跑 demo
cd ../examples/typescript/clients/permit2_exact_bsc_flow
pnpm start
```

---

## 3) Go Solana SVM 一体化

- 路径：`examples/go/svm_exact_solana_flow`

### 必需环境变量

- `SVM_CLIENT_PRIVATE_KEY`：付款方 Solana 私钥（base58）
- `SVM_PAYEE_ADDRESS`：收款地址（base58）
- `SVM_FEE_PAYER`：fee payer 地址（base58）
- `GATE_WEB3_API_KEY`
- `GATE_WEB3_API_SECRET`

### 默认值（已对齐本次测试）

- `SVM_NETWORK=solana-devnet`
- `SVM_ASSET_MINT=BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`
- `PAYMENT_AMOUNT_ATOMIC=100000`（6 位精度，即 0.10）

### 运行

```bash
cd examples/go/svm_exact_solana_flow
go run .
```

### 调试（可选）

```bash
export X402_DEBUG_SUPPORTED=1
export X402_DEBUG_SUPPORTED_RAW=1
go run .
```

---

## 4) TypeScript Solana SVM 一体化

- 路径：`examples/typescript/clients/svm_exact_solana_flow`

### 必需环境变量

- `SVM_CLIENT_PRIVATE_KEY`：付款方私钥（base58 bytes）
- `SVM_PAYEE_ADDRESS`：收款地址（base58）
- `SVM_FEE_PAYER`：fee payer 地址（base58）
- `GATE_WEB3_API_KEY`
- `GATE_WEB3_API_SECRET`

### 默认值（已对齐本次测试）

- `SVM_NETWORK=solana-devnet`
- `SVM_ASSET_MINT=BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`
- `PAYMENT_AMOUNT_ATOMIC=100000`（6 位精度，即 0.10）

### 运行

```bash
# 首次或变更依赖后
cd examples/typescript
pnpm install --no-frozen-lockfile

# 如改过 packages/core / packages/mechanisms/svm 源码，先构建
cd ../../typescript
pnpm build

# 跑 demo
cd ../examples/typescript/clients/svm_exact_solana_flow
pnpm start
```

---

## 一次性串行跑完 4 个示例（手工版）

```bash
# 1) Go Permit2
cd examples/go/clients/permit2_exact_bsc_flow && go run .

# 2) TS Permit2
cd ../../../typescript && pnpm build
cd ../examples/typescript/clients/permit2_exact_bsc_flow && pnpm start

# 3) Go SVM
cd ../../../go/svm_exact_solana_flow && go run .

# 4) TS SVM
cd ../../typescript/clients/svm_exact_solana_flow && pnpm start
```

> 实际执行时建议分开跑，便于查看各自日志与排错。

