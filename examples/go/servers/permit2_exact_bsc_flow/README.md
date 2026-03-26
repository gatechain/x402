# permit2-style exact payment (BSC mainnet)

This is a **server-client** demo for x402 **`exact` scheme** with:
- `assetTransferMethod = "permit2"`
- **BSC mainnet USDT** as the default token

Flow:
1. Client calls `GET /pay` without `PAYMENT-SIGNATURE`
2. Server returns `402` with `PAYMENT-REQUIRED` header
3. Client computes `paymentPayload` (including `permit2Authorization`) + `signature`
4. Client retries `GET /pay` with `PAYMENT-SIGNATURE`
5. Server verifies + calls the facilitator to **settle** on-chain

## Environment variables

- `FACILITATOR_URL` (必填：facilitator 基础地址，例如 `http://localhost:4022`)
- `EVM_PAYEE_ADDRESS` (merchant payTo / witness.to)
- `PERMIT_SPENDER` (x402Permit2Proxy address)
- `PERMIT_NONCE` (default `0`)
- `WITNESS_VALID_AFTER` (default `0`)
- `PERMIT_DEADLINE` (default `now + 3600`)
- `PAYMENT_AMOUNT` (default `1000000000000000000` = 1 USDT with 18 decimals)
- `USDT_ADDRESS` (optional override; default SDK BSC mainnet USDT)
- `SERVER_ADDR` (default `:4023`)

## Run

In one terminal:

```bash
cd examples/go/servers/permit2_exact_bsc_flow
go mod tidy
cp .env-example .env
export FACILITATOR_URL=http://localhost:4022
export EVM_PAYEE_ADDRESS=0x...
export PERMIT_SPENDER=0x...
go run .
```

Then run the client example:

```bash
cd ../../clients/permit2_exact_bsc_flow
go mod tidy
export SERVER_URL=http://localhost:4023/pay
export EVM_PRIVATE_KEY=0x...
go run .
```

