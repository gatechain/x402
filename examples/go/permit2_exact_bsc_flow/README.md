# permit2 exact (BSC) single-main demo

一个 `main.go` 同时包含 **server + client(可选)**。

默认行为：
- 启动 server（在 goroutine 里），然后你可以在别的地方用 `curl` 访问。

可选行为：
- 设置 `RUN_CLIENT=1` 后，进程会在 server ready 后自动跑一遍“402 -> 签名 -> 重试”完整流程。
- 如果你不想触发任何链上交易，可以设置 `MOCK_FACILITATOR=1`：本地 mock facilitator 会返回假 `verify/settle` 结果（`settle` 不会广播交易）。

## 运行

```bash
cd examples/go/permit2_exact_bsc_flow
go mod tidy

# 直接用环境变量注入（不读取 .env 文件）
# 最少建议设置：
export EVM_PRIVATE_KEY=0x...
export EVM_PAYEE_ADDRESS=0xYourPayee

# 可选覆盖（都有默认值）：
# export FACILITATOR_URL=https://openapi-test.gateweb3.cc/api/v1/x402
# export PERMIT_SPENDER=0x3765Cf99CEE0075aFd6Cafe103b1c78Ed75aC9Bf
# export PAYMENT_AMOUNT=100000000000000   # 0.0001 * 1e18
# export RUN_CLIENT=1
# export MOCK_FACILITATOR=1               # 不广播链上交易（dry-run）

go run .
```

启动后用 curl：

```bash
curl -i http://localhost:4023/pay
```

