# TS Demo (Integrated): SVM (Solana) Exact Payment Flow (openapi-test)

This demo runs **server (resource provider)** and **client (payer)** in the same process:

- Local Express server protected by `@x402/express` middleware on `GET /pay`
- In-process client flow: fetch `PAYMENT-REQUIRED`, then retry with `PAYMENT-SIGNATURE`

> Note: Gate openapi-test `/supported` currently does not return `feePayer/signers`, so you must explicitly provide `SVM_FEE_PAYER` (base58 address, not private key).

## Run

```bash
pnpm start
```

Default URL: `http://localhost:4025/pay`

## Environment Variables

- **GATE_WEB3_API_KEY / GATE_WEB3_API_SECRET**: Required (used to call the openapi-test facilitator)
- **FACILITATOR_URL**: Default `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: Default `solana-devnet` (Gate openapi-test verify accepts V1 Solana names)
- **SVM_ASSET_MINT**: Default `BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`
- **PAYMENT_AMOUNT_ATOMIC**: Default `100000` (0.10 for 6-decimal tokens)
- **SVM_PAYEE_ADDRESS**: Required, merchant address (base58)
- **SVM_FEE_PAYER**: Required, fee payer address (base58)
- **SVM_CLIENT_PRIVATE_KEY**: Required, payer private key (base58 bytes)
- **PORT**: Default `4025`
- **ROUTE_PATH**: Default `/pay`
- **KEEP_ALIVE**: Set to `1` to keep server running after client flow

