# permit2 exact (BSC) single-main demo

A single `main.go` contains **server + client (optional)**.

Default behavior:
- Start the server (in a goroutine), then call it from another terminal using `curl`.

Optional behavior:
- Set `RUN_CLIENT=1` and the process will automatically run the full flow after server startup: `402 -> sign -> retry`.
- Set `MOCK_FACILITATOR=1` if you want a no-broadcast dry-run. Local mock facilitator returns stubbed `verify/settle` results and does not submit on-chain settlement.

## Run

```bash
cd examples/go/permit2_exact_bsc_flow
go mod tidy

# Inject via environment variables directly (no .env loading)
# Minimum recommended values:
export EVM_PRIVATE_KEY=0x...
export EVM_PAYEE_ADDRESS=0xYourPayee

# Optional overrides (all have defaults):
# export FACILITATOR_URL=https://openapi-test.gateweb3.cc/api/v1/x402
# export PERMIT_SPENDER=0x3765Cf99CEE0075aFd6Cafe103b1c78Ed75aC9Bf
# export PAYMENT_AMOUNT=100000000000000   # 0.0001 * 1e18
# export RUN_CLIENT=1
# export MOCK_FACILITATOR=1               # no on-chain broadcast (dry-run)

go run .
```

After startup, test with curl:

```bash
curl -i http://localhost:4023/pay
```

