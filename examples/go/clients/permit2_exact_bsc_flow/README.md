# permit2-style exact payment (BSC mainnet) client

This client pairs with:
- `examples/go/servers/permit2_exact_bsc_flow`

It performs:
1. `GET /pay` (expects 402 + `PAYMENT-REQUIRED`)
2. Signs `permit2Authorization` using your `EVM_PRIVATE_KEY`
3. Retries `GET /pay` with `PAYMENT-SIGNATURE`
4. Server verifies + calls facilitator to settle

## Environment variables

- `SERVER_URL` (default `http://localhost:4023/pay`)
- `EVM_PRIVATE_KEY` (payer private key)
- `BSC_NETWORK` (default `eip155:56`)
- `PRINT_PAYLOAD` (optional; if set, prints the generated payment payload)

## Run

```bash
cd examples/go/clients/permit2_exact_bsc_flow
go mod tidy

cp .env-example .env

export SERVER_URL=http://localhost:4023/pay
export EVM_PRIVATE_KEY=0x...

go run .
```

