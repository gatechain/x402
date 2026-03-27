# Go Demo: SVM (Solana) Exact Payment Client

This demo is an **x402 client**: it first requests `PAYMENT-REQUIRED`, then builds a Solana transaction and retries with `PAYMENT-SIGNATURE`.

## Prerequisites

- Start a resource server first (for example `examples/go/servers/svm_exact_solana_flow`)
- Prepare a Solana devnet private key (base58) with enough SOL/tokens for testing

## Run

```bash
go run .
```

Default target: `http://localhost:4024/pay`.

## Environment Variables

- **SERVER_URL**: Default `http://localhost:4024/pay`
- **SVM_NETWORK**: Default `solana-devnet` (also supports CAIP-2, e.g. `solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`)
- **SVM_CLIENT_PRIVATE_KEY**: **Required**, Solana private key (base58)

## Token Configuration Checklist

Provide the following values to finalize defaults and validation:

- Target network (mainnet/devnet/testnet in CAIP-2 form)
- Token mint address
- Token decimals
- Recommended default `PAYMENT_AMOUNT_ATOMIC`

