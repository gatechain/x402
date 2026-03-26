# exact + permit2 + BSC USDT (demo)

Go example that builds an x402 **`scheme=exact`** payment payload with:

- `paymentRequirements.extra.assetTransferMethod = "permit2"`
- EVM payload contains `signature` + `permit2Authorization`:
  - `permit2Authorization.from`
  - `permit2Authorization.spender`
  - `permit2Authorization.permitted.token`
  - `permit2Authorization.permitted.amount`
  - `permit2Authorization.nonce`
  - `permit2Authorization.deadline`
  - `permit2Authorization.witness.to` (`PAYEE_ADDRESS`)
  - `permit2Authorization.witness.validAfter`

Demo signs the `permit2Authorization` structure from the perspective of the **payer** (`EVM_PRIVATE_KEY`).
What happens next on-chain depends on your facilitator/smart contract integration for this `permit2`-style settlement.

## Prerequisites

- BSC mainnet USDT balance for `EVM_PRIVATE_KEY`'s address
- `PERMIT_SPENDER` must be the x402 exact permit2 proxy/spender address expected by your integration

## Run

```bash
cd examples/go/clients/permit2_bsc
go mod tidy

export EVM_PRIVATE_KEY=0x...
export PERMIT_SPENDER=0x...       # goes into permit2Authorization.spender
export PAYEE_ADDRESS=0x...       # goes into permit2Authorization.witness.to

# optional:
#   PAYMENT_AMOUNT (default 1000000000000000000)
#   BSC_NETWORK (default eip155:56)
#   USDT_ADDRESS (override token address)
#   PERMIT_NONCE (default 0)              # placed into paymentPayload.permit2Authorization.nonce
#   PERMIT_DEADLINE (default now+3600)   # placed into paymentPayload.permit2Authorization.deadline
#   WITNESS_VALID_AFTER (default 0)      # placed into paymentPayload.permit2Authorization.witness.validAfter

go run .
```

Default `BSC_NETWORK` is `eip155:56` with SDK default USDT `0x55d398326f99059fF775485246999027B3197955`.

## SDK entry

```go
import (
  x402 "github.com/gatechain/x402/go"
  evm "github.com/gatechain/x402/go/mechanisms/evm"
  exactevmclient "github.com/gatechain/x402/go/mechanisms/evm/exact/client"
  x402evmsigner "github.com/gatechain/x402/go/signers/evm"
)

signer, _ := x402evmsigner.NewClientSignerFromPrivateKey(key)
client := x402.Newx402Client().
  Register("eip155:56", exactevmclient.NewExactEvmScheme(signer))

req := types.PaymentRequirements{
  Scheme:  evm.SchemeExact,
  Network: "eip155:56",
  // ...
  Extra: map[string]interface{}{
    "assetTransferMethod": "permit2",
    "spender": PERMIT_SPENDER,
  },
}
```

BSC chain IDs and default USDT addresses are also registered in `go/mechanisms/evm/constants.go`.
