import { toHex } from "viem";
import { Network } from "@x402/core/types";

/** Gate Layer Testnet chain ID (aligned with Go ChainIDGateLayerTestnet). */
export const CHAIN_ID_GATELAYER_TESTNET = 10087;

/**
 * Resolve EVM chain ID from network string.
 * Supports: eip155:CHAIN_ID, gatelayer_testnet (→ 10087), eip155:10087.
 * Used by V2 exact client for EIP-712 signing.
 *
 * @param network - The network identifier (e.g. "eip155:8453", "gatelayer_testnet")
 * @returns The numeric chain ID
 * @throws Error if network format is unsupported
 */
export function getEvmChainIdFromNetwork(network: string): number {
  const s = network.trim();
  if (s === "gatelayer_testnet" || s === "eip155:10087") {
    return CHAIN_ID_GATELAYER_TESTNET;
  }
  if (s.startsWith("eip155:")) {
    const chainIdStr = s.slice(7).trim();
    const chainId = parseInt(chainIdStr, 10);
    if (Number.isNaN(chainId) || chainIdStr === "" || String(chainId) !== chainIdStr) {
      throw new Error(`unsupported network format: ${network} (expected eip155:CHAIN_ID)`);
    }
    return chainId;
  }
  throw new Error(`unsupported network format: ${network} (expected eip155:CHAIN_ID or gatelayer_testnet)`);
}

/**
 * Extract chain ID from network string (e.g., "base-sepolia" -> 84532)
 * Used by v1 implementations
 *
 * @param network - The network identifier
 * @returns The numeric chain ID
 */
export function getEvmChainId(network: Network): number {
  const networkMap: Record<string, number> = {
    base: 8453,
    "base-sepolia": 84532,
    ethereum: 1,
    sepolia: 11155111,
    polygon: 137,
    "polygon-amoy": 80002,
    gatelayer_testnet: CHAIN_ID_GATELAYER_TESTNET,
  };
  return networkMap[network] || 1;
}

/**
 * Create a random 32-byte nonce for authorization
 *
 * @returns A hex-encoded 32-byte nonce
 */
export function createNonce(): `0x${string}` {
  // Use dynamic import to avoid require() in ESM context
  const cryptoObj =
    typeof globalThis.crypto !== "undefined"
      ? globalThis.crypto
      : (globalThis as { crypto?: Crypto }).crypto;

  if (!cryptoObj) {
    throw new Error("Crypto API not available");
  }

  return toHex(cryptoObj.getRandomValues(new Uint8Array(32)));
}
