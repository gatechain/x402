/**
 * @module @x402/evm - x402 Payment Protocol EVM Implementation
 *
 * This module provides the EVM-specific implementation of the x402 payment protocol.
 */

// Export EVM implementation modules here
// The actual implementation logic will be added by copying from the core/src/schemes/evm folder

export { ExactEvmScheme } from "./exact";
export {
  buildEip712DigestTransferWithAuthorization,
  GATELAYER_TESTNET_USDC_ADDRESS,
  GATELAYER_TESTNET_USDC_DOMAIN_SEPARATOR,
} from "./gatelayer";
export type { TransferWithAuthorizationLike } from "./gatelayer";
export { toClientEvmSigner, toFacilitatorEvmSigner, withSignDigest } from "./signer";
export type { ClientEvmSigner, FacilitatorEvmSigner } from "./signer";
