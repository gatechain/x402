# permit2 exact (BSC) one-click TS server+client

这个示例演示“server 返回 `402 + PAYMENT-REQUIRED` → client 生成 Permit2 签名 → 重试携带 `PAYMENT-SIGNATURE` → server settle”的完整流程。

与之前版本不同：现在 `pnpm start` 会在本进程里同时启动 TS 的受保护 server，并自动发起请求完成一次支付。

由于当前 TypeScript 的 `ExactEvmScheme` 尚未实现 `permit2`（只支持 EIP-3009），因此本示例在 client 侧 **手动构造 `permit2Authorization` + digest + signature**，并按 x402 v2 协议标准生成 `PAYMENT-SIGNATURE`。

## 依赖

本示例会调用 Gate Web3 OpenAPI facilitator（默认 `https://openapi-test.gateweb3.cc/api/v1/x402`），因此需要你在环境变量里配置 Gate 的鉴权信息。

## 环境变量
- `EVM_PRIVATE_KEY`：payer EOA 私钥（用于签名 permit2 digest）
- `EVM_PAYEE_ADDRESS`：payee/merchant 地址（用于 `witness.to` / `payTo`）
- `GATE_WEB3_API_KEY`、`GATE_WEB3_API_SECRET`：**必填**。访问默认 facilitator（openapi-test）时用于 HMAC 签名；缺省会 `401 missing access key`，随后初始化报错看起来像「不支持 bsc」（与 Go 示例一致）
- `GATE_WEB3_PASSPHRASE`、`GATE_WEB3_REAL_IP`：按你 Gate 控制台 / 文档要求可选配置

可选：
- `FACILITATOR_URL`：facilitator URL（默认 openapi-test）
- `PORT`：本地 server 端口（默认 `4023`）
- `PERMIT_SPENDER`：permit2 proxy spender（默认使用本仓库 Go demo 的值）
- `PERMIT_NONCE`、`WITNESS_VALID_AFTER`：默认 `0`
- `PERMIT_DEADLINE`：默认 `当前时间+3600s`
- `PAYMENT_AMOUNT`：默认 `100000000000000`（0.0001 USDT，18 decimals）

## 首次安装（必须）

本目录属于 `examples/typescript` 的 pnpm workspace，**不能直接只在这个子目录装依赖**（否则没有 `node_modules`，会出现 `tsx: command not found`）。请先在 examples 根目录安装并编译 workspace 包（`@x402/*` 从 `dist` 导出）：

```bash
cd examples/typescript
pnpm install
pnpm build
```

如果 `pnpm install` 报 `ERR_PNPM_OUTDATED_LOCKFILE`（lockfile 与 workspace 里某个 `package.json` 不一致），用：

```bash
pnpm install --no-frozen-lockfile
```

## 运行

确保环境变量已配置好（尤其是 `EVM_PRIVATE_KEY` / `EVM_PAYEE_ADDRESS` / `GATE_WEB3_API_KEY` / `GATE_WEB3_API_SECRET`），然后：

```bash
cd examples/typescript/clients/permit2_exact_bsc_flow
pnpm start
```

运行时会自动：
- 发起第一次请求获取 `PAYMENT-REQUIRED`
- 生成 permit2 签名
- 用 `PAYMENT-SIGNATURE` 重试
- 打印 `PAYMENT-RESPONSE`（如果返回）

## 排错

若出现 `missing access key` 或 `Facilitator does not support scheme "exact" on network "bsc"`：先检查是否已 `export GATE_WEB3_API_KEY` / `GATE_WEB3_API_SECRET`；后者往往是前者的连锁误判，并非真的不支持 BSC。

若 **shell 里已能 `echo` 出 AK/SK 仍报 `missing access key`**：请重新编译 `@x402/core`（`HTTPFacilitatorClient` 在 ESM/tsx 下不能用 `require('node:crypto')`，需更新到使用 `import('node:crypto')` 的版本）：

```bash
cd examples/typescript && pnpm build --filter @x402/core
```

或全量 `pnpm build`。

