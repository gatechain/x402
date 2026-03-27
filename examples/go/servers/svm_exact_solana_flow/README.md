# Go Demo: x402 Exact on Solana (SVM)

This demo is an **x402 resource server** (Gin middleware) using the **`exact`** scheme on Solana.

It will:

- Return `402` on `GET /pay`, with `accepts[]` in the `PAYMENT-REQUIRED` header
- Allow access after the client retries with `PAYMENT-SIGNATURE`, then verify + settle via facilitator

## Run

```bash
go run .
```

Default listen URL: `http://localhost:4024/pay`.

## Environment Variables

- **FACILITATOR_URL**: Default `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: Default `solana-devnet` (also supports CAIP-2, e.g. `solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`)
- **SVM_PAYEE_ADDRESS**: **Required**, Solana payee address (base58)
- **SVM_ASSET_MINT**: Token mint (base58), default openapi-test devnet mint (`BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`)
- **PAYMENT_AMOUNT_ATOMIC**: Atomic amount (minimum unit string), default `100000` (0.10 for 6-decimal tokens)
- **PORT**: Default `4024`
- **ROUTE_PATH**: Default `/pay`

After mint/decimals/default amount/network are finalized, update defaults and validation accordingly.

