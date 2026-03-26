import { encodeAbiParameters, keccak256, concatHex, stringToBytes, hexToBigInt } from "viem";
import { createSignerFromPrivateKey } from "@x402/evm";
import {
  decodePaymentRequiredHeader,
  decodePaymentResponseHeader,
  encodePaymentSignatureHeader,
} from "@x402/core/http";
import type { PaymentPayload, PaymentRequired, PaymentRequirements } from "@x402/core/types";

const SERVER_URL = process.env.SERVER_URL ?? "http://localhost:4023/pay";

// Payer EOA private key used to sign Permit2 permitWitnessTransferFrom digest.
const EVM_PRIVATE_KEY = process.env.EVM_PRIVATE_KEY as `0x${string}` | undefined;
if (!EVM_PRIVATE_KEY) {
  console.error("EVM_PRIVATE_KEY required");
  process.exit(1);
}

// The Go server should already return PaymentRequired for permit2 exact on BSC.
const permit2CanonicalAddress = "0x000000000022D473030F116dDEE9F6B43aC78BA3";

/**
 * Read an extra field as a decimal string.
 *
 * @param extra - The `requirements.extra` object
 * @param key - Extra key to read
 * @returns The value as a decimal string
 */
function getExtraString(extra: Record<string, unknown>, key: string): string {
  const v = extra[key];
  if (typeof v === "string") return v;
  if (typeof v === "number") return String(v);
  if (typeof v === "bigint") return v.toString();
  throw new Error(`missing/invalid extra.${key}: ${String(v)}`);
}

// We need chainId for Permit2 EIP712 domain separator.
/**
 * Resolve EVM chainId for Permit2 domain separator.
 *
 * @param network - x402 network string (e.g. "bsc", "eip155:56")
 * @returns Chain ID as bigint
 */
function getPermit2ChainIdFromNetwork(network: string): bigint {
  // Align with Go's evm.GetEvmChainId:
  // bsc/bnb -> 56, bsc-testnet -> 97
  if (network === "bsc" || network === "bnb") return 56n;
  if (network === "bsc-testnet" || network === "bnb-testnet") return 97n;
  if (network.startsWith("eip155:")) return BigInt(network.slice("eip155:".length));
  throw new Error(`unsupported network for permit2 chainId: ${network}`);
}

/**
 * keccak256 UTF-8 string helper.
 *
 * @param s - Input string
 * @returns 0x-prefixed keccak256 hash
 */
function keccakString(s: string): `0x${string}` {
  return keccak256(stringToBytes(s)) as `0x${string}`;
}

/**
 * Build the Permit2 EIP-712 digest that signs `permitWitnessTransferFrom`.
 *
 * @param args - Permit2 inputs
 * @returns Digest as a 0x-prefixed hex string
 */
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
  // tokenPermissionsHash = keccak256(abi.encode(tokenPermissionsTypehash, token, permittedAmount))
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

  // permitHash = keccak256(abi.encode(typeHash, tokenPermissionsHash, proxySpender, nonce, deadline, witnessHash))
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

  // Domain separator: keccak256(abi.encode(domainTypeHash, keccak256("Permit2"), chainId, permit2CanonicalAddress))
  const domainTypeHash = keccakString(
    "EIP712Domain(string name,uint256 chainId,address verifyingContract)",
  );
  const nameHash = keccakString("Permit2");
  const domainSeparator = keccak256(
    encodeAbiParameters(
      [
        { type: "bytes32" },
        { type: "bytes32" },
        { type: "uint256" },
        { type: "address" },
      ],
      [domainTypeHash, nameHash, args.chainId, permit2CanonicalAddress as `0x${string}`],
    ),
  ) as `0x${string}`;

  // digest = keccak256(0x1901 || domainSeparator || permitHash)
  const digest = keccak256(concatHex(["0x1901", domainSeparator, permitHash])) as `0x${string}`;
  return digest;
}

/**
 * Construct a full x402 v2 `PaymentPayload` with permit2 signature.
 *
 * @param paymentRequired - Decoded PAYMENT-REQUIRED (v2)
 * @param selected - Chosen payment requirement (scheme/network)
 * @returns A full PAYMENT payload object ready for `encodePaymentSignatureHeader`
 */
async function buildPermit2PaymentPayload(
  paymentRequired: PaymentRequired,
  selected: PaymentRequirements,
): Promise<PaymentPayload> {
  const extra = selected.extra as Record<string, unknown>;
  if (!extra || typeof extra !== "object") {
    throw new Error("payment requirements extra missing");
  }

  const assetTransferMethod = extra["assetTransferMethod"];
  if (typeof assetTransferMethod !== "string" || assetTransferMethod.toLowerCase() !== "permit2") {
    throw new Error(`assetTransferMethod must be permit2, got: ${String(assetTransferMethod)}`);
  }

  const proxySpender = getExtraString(extra, "spender") as `0x${string}`;
  const nonceStr = getExtraString(extra, "permitNonce");
  const deadlineStr = getExtraString(extra, "deadline");
  const validAfterStr = getExtraString(extra, "validAfter");

  const nonce = hexToBigInt(`0x${BigInt(nonceStr).toString(16)}`);
  const deadline = hexToBigInt(`0x${BigInt(deadlineStr).toString(16)}`);
  const validAfter = hexToBigInt(`0x${BigInt(validAfterStr).toString(16)}`);

  const chainId = getPermit2ChainIdFromNetwork(String(selected.network));

  const amount = BigInt(selected.amount);
  const witnessTo = selected.payTo as `0x${string}`;

  const digest = buildPermit2WitnessDigest({
    chainId,
    proxySpender,
    tokenAddr: selected.asset as `0x${string}`,
    permittedAmount: amount,
    nonce,
    deadline,
    witnessTo,
    validAfter,
  });

  const signer = createSignerFromPrivateKey(EVM_PRIVATE_KEY);
  if (!signer.signDigest) {
    throw new Error("signer.signDigest missing");
  }

  const signature = await signer.signDigest(digest);

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

  return paymentPayload;
}

/**
 * Execute the handshake:
 * 1) GET (expect 402 + PAYMENT-REQUIRED)
 * 2) sign permit2 digest
 * 3) GET retry with PAYMENT-SIGNATURE
 *
 * @returns Resolves when the script finishes
 */
async function main(): Promise<void> {
  console.log(`Requesting: ${SERVER_URL}`);

  const initial = await fetch(SERVER_URL, { method: "GET" });
  console.log(`Initial status=${initial.status}`);

  if (initial.status !== 402) {
    const txt = await initial.text().catch(() => "");
    console.log("Response body:", txt);
    return;
  }

  const paymentRequiredHeader = initial.headers.get("PAYMENT-REQUIRED");
  if (!paymentRequiredHeader) throw new Error("Missing PAYMENT-REQUIRED header");

  const paymentRequired = decodePaymentRequiredHeader(paymentRequiredHeader) as PaymentRequired;
  const selected = paymentRequired.accepts.find(
    (a) => a.scheme === "exact" && String(a.network) === "bsc",
  );
  if (!selected) {
    throw new Error(`No matching accepted requirement (exact+bsc). accepts=${JSON.stringify(paymentRequired.accepts)}`);
  }

  console.log("Selected requirement:", {
    scheme: selected.scheme,
    network: selected.network,
    asset: selected.asset,
    amount: selected.amount,
    payTo: selected.payTo,
    extraKeys: Object.keys(selected.extra ?? {}),
  });

  const paymentPayload = await buildPermit2PaymentPayload(paymentRequired, selected);
  const paymentSignatureHeader = encodePaymentSignatureHeader(paymentPayload);

  const retry = await fetch(SERVER_URL, {
    method: "GET",
    headers: {
      "PAYMENT-SIGNATURE": paymentSignatureHeader,
    },
  });

  console.log(`Retry status=${retry.status}`);
  const bodyText = await retry.text().catch(() => "");
  console.log("Retry body:", bodyText);

  const paymentResponseHeader = retry.headers.get("PAYMENT-RESPONSE");
  if (paymentResponseHeader) {
    const paymentResponse = decodePaymentResponseHeader(paymentResponseHeader);
    console.log("PAYMENT-RESPONSE:", paymentResponse);
  }
}

main().catch((e) => {
  console.error("Error:", e instanceof Error ? e.message : String(e));
  process.exit(1);
});

