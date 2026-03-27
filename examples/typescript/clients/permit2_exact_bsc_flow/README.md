# Permit2 Exact (BSC) One-Click TS Server + Client

This example demonstrates the full flow:
`server returns 402 + PAYMENT-REQUIRED` -> `client creates a Permit2 signature` -> `client retries with PAYMENT-SIGNATURE` -> `server settles`.

Unlike older versions, `pnpm start` now launches the protected TS server and runs the client request in the same process.

Because TypeScript `ExactEvmScheme` currently does not implement Permit2 directly (EIP-3009 only), this example **manually builds `permit2Authorization` + digest + signature** on the client side and sends a v2-compliant `PAYMENT-SIGNATURE`.

## Dependencies

This example calls the Gate Web3 OpenAPI facilitator (default: `https://openapi-test.gateweb3.cc/api/v1/x402`), so Gate authentication environment variables are required.

## Environment Variables

- `EVM_PRIVATE_KEY`: Payer EOA private key (used to sign the Permit2 digest)
- `EVM_PAYEE_ADDRESS`: Payee/merchant address (used for `witness.to` / `payTo`)
- `GATE_WEB3_API_KEY`, `GATE_WEB3_API_SECRET`: **Required**. Used for HMAC signing against the default facilitator (openapi-test). If missing, you may see `401 missing access key`, followed by an initialization error that looks like unsupported BSC.
- `GATE_WEB3_PASSPHRASE`, `GATE_WEB3_REAL_IP`: Optional, depending on your Gate console / docs configuration

Optional:

- `FACILITATOR_URL`: Facilitator URL (default: openapi-test)
- `PORT`: Local server port (default: `4023`)
- `PERMIT_SPENDER`: Permit2 proxy spender (defaults to the value used in this repo's Go demo)
- `PERMIT_NONCE`, `WITNESS_VALID_AFTER`: default `0`
- `PERMIT_DEADLINE`: default `now + 3600s`
- `PAYMENT_AMOUNT`: default `100000000000000` (0.0001 USDT, 18 decimals)

## First-Time Setup (Required)

This directory is part of the `examples/typescript` pnpm workspace.  
**Do not install dependencies only inside this subdirectory** (otherwise `node_modules` will be missing and you may get `tsx: command not found`).

Install and build from the examples root first (`@x402/*` packages are consumed from `dist`):

```bash
cd examples/typescript
pnpm install
pnpm build
```

If `pnpm install` fails with `ERR_PNPM_OUTDATED_LOCKFILE`, run:

```bash
pnpm install --no-frozen-lockfile
```

## Run

After setting environment variables (especially `EVM_PRIVATE_KEY`, `EVM_PAYEE_ADDRESS`, `GATE_WEB3_API_KEY`, `GATE_WEB3_API_SECRET`):

```bash
cd examples/typescript/clients/permit2_exact_bsc_flow
pnpm start
```

The script will automatically:

- Send the first request and read `PAYMENT-REQUIRED`
- Build a Permit2 signature
- Retry with `PAYMENT-SIGNATURE`
- Print `PAYMENT-RESPONSE` (if present)

## Troubleshooting

If you see `missing access key` or `Facilitator does not support scheme "exact" on network "bsc"`:
first verify that `GATE_WEB3_API_KEY` / `GATE_WEB3_API_SECRET` are exported. The latter message is often a chained symptom of missing credentials, not actual BSC unsupported behavior.

If `echo` confirms AK/SK in your shell but you still get `missing access key`, rebuild `@x402/core`:

```bash
cd examples/typescript
pnpm build --filter @x402/core
```

Or run full `pnpm build`.

