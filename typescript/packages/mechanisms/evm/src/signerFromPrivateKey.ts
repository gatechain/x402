/**
 * Create a ClientEvmSigner from a private key with correct signDigest for gatelayer_testnet.
 * Signs the raw 32-byte EIP-712 digest (no personal_sign prefix) so verification succeeds.
 */
import { signAsync } from "@noble/secp256k1";
import { type Hex, hexToBytes } from "viem";
import { privateKeyToAccount } from "viem/accounts";
import type { ClientEvmSigner } from "./signer";

const toHex = (bytes: Uint8Array): string =>
  Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");

/**
 * Sign a 32-byte EIP-712 digest with the given private key (raw ECDSA, no 0x19 prefix).
 * Returns signature as 0x + r (32 bytes hex) + s (32 bytes hex) + v (27 or 28).
 */
export async function signDigestWithPrivateKey(
  digest: Hex,
  privateKey: Hex,
): Promise<`0x${string}`> {
  const msg = hexToBytes(digest);
  if (msg.length !== 32) {
    throw new Error(`Digest must be 32 bytes, got ${msg.length}`);
  }
  const key = hexToBytes(privateKey.startsWith("0x") ? privateKey : `0x${privateKey}`);
  // prehash: false = digest is already the hash. @noble/secp256k1 v2 returns RecoveredSignature (object with r, s, recovery).
  const sig = await signAsync(msg, key, { prehash: false });
  const compact = sig.toCompactRawBytes();
  if (compact.length !== 64) {
    throw new Error(`Expected 64-byte compact signature, got ${compact.length}`);
  }
  const recovery = (sig as { recovery?: number }).recovery ?? 0;
  const v = 27 + (recovery & 1); // Ethereum v is 27 or 28
  const rHex = toHex(compact.slice(0, 32));
  const sHex = toHex(compact.slice(32, 64));
  const vHex = v.toString(16).padStart(2, "0");
  return `0x${rHex}${sHex}${vHex}` as `0x${string}`;
}

/**
 * Create a ClientEvmSigner from a private key that works for both normal EIP-712
 * and gatelayer_testnet tokens (uses raw digest signing for the latter).
 *
 * Use this when you need to pay with gatelayer_testnet tokens and were getting
 * "invalid signature" — viem's account.sign({ hash }) may add a prefix; this
 * signs the digest correctly so verification succeeds.
 *
 * @param privateKey - Hex private key (with or without 0x)
 * @returns ClientEvmSigner with signTypedData (viem) and signDigest (raw ECDSA)
 */
export function createSignerFromPrivateKey(privateKey: Hex): ClientEvmSigner {
  const key = privateKey.startsWith("0x") ? privateKey : (`0x${privateKey}` as Hex);
  const account = privateKeyToAccount(key as `0x${string}`);
  return {
    address: account.address,
    signTypedData: (msg) => account.signTypedData(msg),
    signDigest: (digest) => signDigestWithPrivateKey(digest, key),
  };
}
