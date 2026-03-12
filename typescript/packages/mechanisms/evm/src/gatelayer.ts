/**
 * Gate Layer Testnet USDC: use chain's DOMAIN_SEPARATOR for EIP-3009 signing
 * so signatures verify on-chain. Aligned with Go exact/client/scheme.go.
 */
import { type Hex, concatHex, keccak256, padHex, toHex } from "viem";

/** USDC on Gate Layer Testnet (same as Go NetworkConfigs). */
export const GATELAYER_TESTNET_USDC_ADDRESS = "0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF".toLowerCase();

/** DOMAIN_SEPARATOR from chain for that USDC contract (Go hardcoded value). */
export const GATELAYER_TESTNET_USDC_DOMAIN_SEPARATOR: Hex =
  "0x2c2d6b621e73a4a094449d1894717413742130fb20149ec48340ca0354d1a707";

const TRANSFER_WITH_AUTHORIZATION_TYPE =
  "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)";

export interface TransferWithAuthorizationLike {
  from: string;
  to: string;
  value: string;
  validAfter: string;
  validBefore: string;
  nonce: Hex;
}

/** EIP-712 type hash for TransferWithAuthorization (keccak256 of type string UTF-8). */
function transferWithAuthorizationTypeHash(): Hex {
  const bytes = new TextEncoder().encode(TRANSFER_WITH_AUTHORIZATION_TYPE);
  return keccak256(new Uint8Array(bytes));
}

/**
 * Build EIP-712 digest for TransferWithAuthorization using a precomputed domain separator.
 * digest = keccak256("\x19\x01" || domainSeparator || structHash)
 */
export function buildEip712DigestTransferWithAuthorization(
  domainSeparator: Hex,
  authorization: TransferWithAuthorizationLike,
): Hex {
  const typeHash = transferWithAuthorizationTypeHash();

  const encoded = concatHex([
    typeHash,
    padHex(authorization.from as Hex, { size: 32 }),
    padHex(authorization.to as Hex, { size: 32 }),
    padHex(toHex(BigInt(authorization.value)), { size: 32 }),
    padHex(toHex(BigInt(authorization.validAfter)), { size: 32 }),
    padHex(toHex(BigInt(authorization.validBefore)), { size: 32 }),
    authorization.nonce.length === 66 ? (authorization.nonce as Hex) : padHex(authorization.nonce as Hex, { size: 32 }),
  ]);

  const structHash = keccak256(encoded);
  const digest = keccak256(concatHex(["0x1901", domainSeparator, structHash]));
  return digest;
}
