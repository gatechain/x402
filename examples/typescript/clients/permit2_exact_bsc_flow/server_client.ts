import express from "express";
import { paymentMiddleware, x402ResourceServer } from "@x402/express";
import { HTTPFacilitatorClient } from "@x402/core/server";
import { ExactEvmScheme } from "@x402/evm/exact/server";
import {
  decodePaymentRequiredHeader,
  decodePaymentResponseHeader,
  encodePaymentSignatureHeader,
} from "@x402/core/http";
import type { PaymentPayload, PaymentRequired, PaymentRequirements } from "@x402/core/types";
import { createSignerFromPrivateKey } from "@x402/evm";
import {
  encodeAbiParameters,
  keccak256,
  concatHex,
  stringToBytes,
  hexToBigInt,
  type Hex,
} from "viem";

const PORT = Number(process.env.PORT ?? 4023);
const ROUTE_PATH = "/pay";
const SERVER_URL = `http://localhost:${PORT}${ROUTE_PATH}`;

const EVM_PRIVATE_KEY = process.env.EVM_PRIVATE_KEY as `0x${string}` | undefined;
if (!EVM_PRIVATE_KEY) {
  console.error("EVM_PRIVATE_KEY required");
  process.exit(1);
}

const EVM_PAYEE_ADDRESS = process.env.EVM_PAYEE_ADDRESS as `0x${string}` | undefined;
if (!EVM_PAYEE_ADDRESS) {
  console.error("EVM_PAYEE_ADDRESS required");
  process.exit(1);
}

function requireGateOpenapiCredentials(): void {
  const ak = (process.env.GATE_WEB3_API_KEY ?? "").trim();
  const sk = (process.env.GATE_WEB3_API_SECRET ?? "").trim();
  if (ak && sk) return;
  console.error(
    [
      "GATE_WEB3_API_KEY and GATE_WEB3_API_SECRET are required.",
      "Without them, facilitator getSupported returns 401 (missing access key),",
      "then route init fails with a misleading: facilitator does not support exact on bsc.",
      "Use the same env vars as examples/go/permit2_exact_bsc_flow.",
    ].join(" "),
  );
  process.exit(1);
}

const PERMIT_SPENDER =
  (process.env.PERMIT_SPENDER as `0x${string}` | undefined) ??
  ("0x701cCFfcdE34b92B16599Ac865AA1395A1a1F38c" as `0x${string}`);

// BSC mainnet USDT (matches Go demo logs)
const USDT_BSC_ADDRESS = "0x55d398326f99059fF775485246999027B3197955" as `0x${string}`;

const PAYMENT_AMOUNT = process.env.PAYMENT_AMOUNT ?? "100000000000000"; // 0.0001 USDT (18 decimals)

// Permit2 witness timing (unix seconds)
function randomUint256DecimalString(): string {
  // Prefer WebCrypto (Node 20+). Fallback to a timestamp-based nonce.
  const cryptoObj =
    typeof globalThis.crypto !== "undefined"
      ? globalThis.crypto
      : (globalThis as { crypto?: Crypto }).crypto;

  if (!cryptoObj?.getRandomValues) {
    // Still better than "0": avoids immediate nonce reuse across runs.
    return String(BigInt(Date.now()) * 1_000_000n + BigInt(Math.floor(Math.random() * 1_000_000)));
  }

  const bytes = cryptoObj.getRandomValues(new Uint8Array(32));
  let hex = "0x";
  for (const b of bytes) hex += b.toString(16).padStart(2, "0");
  return BigInt(hex).toString(10);
}

// IMPORTANT: Permit2 uses an unordered nonce bitmap; reusing the same nonce (e.g. "0")
// will fail with "nonce already used". Default to a fresh random nonce per run.
const PERMIT_NONCE = (process.env.PERMIT_NONCE ?? "").trim() || randomUint256DecimalString();
const WITNESS_VALID_AFTER = process.env.WITNESS_VALID_AFTER ?? "0";
const PERMIT_DEADLINE =
  process.env.PERMIT_DEADLINE ?? String(Math.floor(Date.now() / 1000) + 3600);

// Gate facilitator (must support exact+bsc+permit2). Default is openapi-test.
const FACILITATOR_URL = process.env.FACILITATOR_URL;

function getPermit2ChainIdFromNetwork(network: string): bigint {
  // Align with Go's evm.GetEvmChainId:
  // bsc/bnb -> 56, bsc-testnet -> 97
  if (network === "bsc" || network === "bnb") return 56n;
  if (network === "bsc-testnet" || network === "bnb-testnet") return 97n;
  if (network.startsWith("eip155:")) return BigInt(network.slice("eip155:".length));
  throw new Error(`unsupported network for permit2 chainId: ${network}`);
}

function keccakString(s: string): `0x${string}` {
  return keccak256(stringToBytes(s)) as `0x${string}`;
}

function getExtraString(extra: Record<string, unknown>, key: string): string {
  const v = extra[key];
  if (typeof v === "string") return v;
  if (typeof v === "number") return String(v);
  if (typeof v === "bigint") return v.toString();
  throw new Error(`missing/invalid extra.${key}: ${String(v)}`);
}

function buildPermit2WitnessDigest(args: {
  chainId: bigint;
  proxySpender: `0x${string}`;
  tokenAddr: `0x${string}`;
  permittedAmount: bigint;
  nonce: bigint;
  deadline: bigint;
  witnessTo: `0x${string}`;
  validAfter: bigint;
}): `0x${string}` {
  // Must match go/mechanisms/evm/exact/facilitator/scheme.go computePermit2WitnessDigest
  const witnessTypeString =
    "Witness witness)TokenPermissions(address token,uint256 amount)Witness(address to,uint256 validAfter)";
  const witnessTypeHash = keccakString("Witness(address to,uint256 validAfter)");

  const witnessHash = keccak256(
    encodeAbiParameters(
      [
        { type: "bytes32" },
        { type: "address" },
        { type: "uint256" },
      ],
      [witnessTypeHash, args.witnessTo, args.validAfter],
    ),
  ) as `0x${string}`;

  const tokenPermissionsTypehash = keccakString("TokenPermissions(address token,uint256 amount)");
  const tokenPermissionsHash = keccak256(
    encodeAbiParameters(
      [
        { type: "bytes32" },
        { type: "address" },
        { type: "uint256" },
      ],
      [tokenPermissionsTypehash, args.tokenAddr, args.permittedAmount],
    ),
  ) as `0x${string}`;

  const permitTransferFromWitnessTypeHashStub =
    "PermitWitnessTransferFrom(TokenPermissions permitted,address spender,uint256 nonce,uint256 deadline,";
  const typeHash = keccakString(permitTransferFromWitnessTypeHashStub + witnessTypeString);

  const permitHash = keccak256(
    encodeAbiParameters(
      [
        { type: "bytes32" },
        { type: "bytes32" },
        { type: "address" },
        { type: "uint256" },
        { type: "uint256" },
        { type: "bytes32" },
      ],
      [typeHash, tokenPermissionsHash, args.proxySpender, args.nonce, args.deadline, witnessHash],
    ),
  ) as `0x${string}`;

  const domainTypeHash = keccakString(
    "EIP712Domain(string name,uint256 chainId,address verifyingContract)",
  );
  const nameHash = keccakString("Permit2");
  const permit2CanonicalAddress = "0x000000000022D473030F116dDEE9F6B43aC78BA3" as `0x${string}`;

  const domainSeparator = keccak256(
    encodeAbiParameters(
      [
        { type: "bytes32" },
        { type: "bytes32" },
        { type: "uint256" },
        { type: "address" },
      ],
      [domainTypeHash, nameHash, args.chainId, permit2CanonicalAddress],
    ),
  ) as `0x${string}`;

  // digest = keccak256(0x1901 || domainSeparator || permitHash)
  const digest = keccak256(concatHex(["0x1901", domainSeparator, permitHash])) as `0x${string}`;
  return digest;
}

async function runClient(): Promise<void> {
  const initial = await fetch(SERVER_URL, { method: "GET" });
  console.log(`TS client initial status=${initial.status}`);

  if (initial.status !== 402) {
    console.log("Initial body:", await initial.text().catch(() => ""));
    return;
  }

  const paymentRequiredHeader = initial.headers.get("PAYMENT-REQUIRED");
  if (!paymentRequiredHeader) throw new Error("Missing PAYMENT-REQUIRED");

  const paymentRequired = decodePaymentRequiredHeader(paymentRequiredHeader) as PaymentRequired;
  const selected = paymentRequired.accepts.find(
    (a) => a.scheme === "exact" && String(a.network) === "bsc",
  ) as PaymentRequirements | undefined;
  if (!selected) {
    throw new Error(
      `No accepted requirement exact+bsc. accepts=${JSON.stringify(paymentRequired.accepts)}`,
    );
  }

  const extra = selected.extra as Record<string, unknown>;
  const proxySpender = getExtraString(extra, "spender") as `0x${string}`;
  const nonceStr = getExtraString(extra, "permitNonce");
  const deadlineStr = getExtraString(extra, "deadline");
  const validAfterStr = getExtraString(extra, "validAfter");

  const nonce = hexToBigInt(`0x${BigInt(nonceStr).toString(16)}`);
  const deadline = hexToBigInt(`0x${BigInt(deadlineStr).toString(16)}`);
  const validAfter = hexToBigInt(`0x${BigInt(validAfterStr).toString(16)}`);

  const chainId = getPermit2ChainIdFromNetwork(String(selected.network));
  const amount = BigInt(selected.amount);

  const digest = buildPermit2WitnessDigest({
    chainId,
    proxySpender,
    tokenAddr: selected.asset as `0x${string}`,
    permittedAmount: amount,
    nonce,
    deadline,
    witnessTo: selected.payTo as `0x${string}`,
    validAfter,
  });

  const signer = createSignerFromPrivateKey(EVM_PRIVATE_KEY);
  if (!signer.signDigest) throw new Error("signer.signDigest missing");
  const signature = await signer.signDigest(digest as Hex);

  const paymentPayload: PaymentPayload = {
    x402Version: paymentRequired.x402Version,
    resource: paymentRequired.resource,
    accepted: selected,
    payload: {
      signature,
      permit2Authorization: {
        from: signer.address,
        spender: proxySpender,
        permitted: {
          token: selected.asset,
          amount: selected.amount,
        },
        nonce: nonceStr,
        deadline: deadlineStr,
        witness: {
          to: selected.payTo,
          validAfter: validAfterStr,
        },
      },
    },
  };

  const paymentHeader = encodePaymentSignatureHeader(paymentPayload);
  console.log("TS client retry with PAYMENT-SIGNATURE...");

  const retry = await fetch(SERVER_URL, {
    method: "GET",
    headers: { "PAYMENT-SIGNATURE": paymentHeader },
  });

  console.log(`TS retry status=${retry.status}`);
  const retryBodyText = await retry.text().catch(() => "");
  console.log("TS retry body:", retryBodyText);

  if (retry.status === 402) {
    const again = retry.headers.get("PAYMENT-REQUIRED");
    if (again) {
      try {
        const pr = decodePaymentRequiredHeader(again) as PaymentRequired;
        console.log("TS retry PAYMENT-REQUIRED error:", pr.error ?? "(none)");
        console.log("TS retry PAYMENT-REQUIRED accepts[0].extra:", pr.accepts?.[0]?.extra ?? "(none)");
      } catch {
        console.log("TS retry PAYMENT-REQUIRED: (decode failed)");
      }
    }
  }

  const paymentResponseHeader = retry.headers.get("PAYMENT-RESPONSE");
  if (paymentResponseHeader) {
    const paymentResponse = decodePaymentResponseHeader(paymentResponseHeader);
    console.log("PAYMENT-RESPONSE:", paymentResponse);
  }
}

async function main(): Promise<void> {
  requireGateOpenapiCredentials();
  const facilitatorClient = new HTTPFacilitatorClient({ url: FACILITATOR_URL });
  const resourceServer = new x402ResourceServer(facilitatorClient).register("bsc", new ExactEvmScheme());

  const app = express();

  const extra = {
    assetTransferMethod: "permit2",
    spender: PERMIT_SPENDER,
    permitNonce: PERMIT_NONCE,
    deadline: PERMIT_DEADLINE,
    validAfter: WITNESS_VALID_AFTER,
  };

  app.use(
    paymentMiddleware(
      {
        [`GET ${ROUTE_PATH}`]: {
          accepts: {
            scheme: "exact",
            price: {
              amount: PAYMENT_AMOUNT,
              asset: USDT_BSC_ADDRESS,
              extra,
            },
            network: "bsc",
            payTo: EVM_PAYEE_ADDRESS,
            maxTimeoutSeconds: 60,
          },
          description: "permit2-style exact payment (BSC)",
          mimeType: "application/json",
        },
      },
      resourceServer,
    ),
  );

  app.get(ROUTE_PATH, (_req, res) => {
    res.json({ ok: true, route: ROUTE_PATH, network: "bsc" });
  });

  const server = app.listen(PORT, async () => {
    console.log(`TS x402 server listening ${SERVER_URL}`);
    try {
      await runClient();
    } finally {
      if (process.env.KEEP_ALIVE !== "1") server.close();
    }
  });
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});

