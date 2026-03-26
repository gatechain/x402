# 新增 SDK 用法汇总（Permit2 + Solana SVM）

本文档汇总本次新增/改造的 4 个示例：

1. Go - Permit2(BSC) 客户端
2. TypeScript - Permit2(BSC) 一体化（server+client）
3. Go - Solana SVM 一体化（server+client）
4. TypeScript - Solana SVM 一体化（server+client）

> 安全提醒：私钥只放本地环境变量或 `.env`，不要提交到仓库，也不要发到聊天记录。

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

