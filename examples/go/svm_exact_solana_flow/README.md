# Go Demo (Integrated): SVM (Solana) Exact Payment Flow

This demo combines **server (resource side)** and **client (payer side)** in one `main.go`:

- Start local Gin + x402 middleware on `GET /pay`
- Run client in-process: fetch `PAYMENT-REQUIRED`, then retry with `PAYMENT-SIGNATURE`
- Exit after one run by default; set `KEEP_ALIVE=1` to keep server running

## Run

```bash
go run .
```

Default endpoint: `http://localhost:4024/pay`

## Environment Variables

- **FACILITATOR_URL**: Default `https://openapi-test.gateweb3.cc/api/v1/x402`
- **SVM_NETWORK**: Default `solana-devnet`. CAIP-2 is also accepted (for example `solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1`), and this demo normalizes to the V1 network name for openapi-test verify.
- **SVM_PAYEE_ADDRESS**: **Required**, Solana payee address (base58)
- **SVM_ASSET_MINT**: Default openapi-test devnet mint (`BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3`)
- **PAYMENT_AMOUNT_ATOMIC**: Default `100000` (0.10 for 6-decimal tokens)
- **SVM_FEE_PAYER**: Solana fee payer (base58). Since openapi-test `/supported` currently omits `feePayer/signers`, provide this explicitly.
- **SVM_CLIENT_PRIVATE_KEY**: **Required**, Solana private key (base58)
- **PORT**: Default `4024`
- **ROUTE_PATH**: Default `/pay`
- **KEEP_ALIVE**: Set to `1` to keep server running after client flow

