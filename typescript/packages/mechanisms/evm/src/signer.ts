/**
 * ClientEvmSigner - Used by x402 clients to sign payment authorizations
 * This is typically a LocalAccount or wallet that holds private keys
 * and can sign EIP-712 typed data for payment authorizations.
 *
 * Optional signDigest: when provided, used for chains where the token's
 * DOMAIN_SEPARATOR must be used exactly (e.g. Gate Layer Testnet USDC).
 * The signer must sign the 32-byte EIP-712 digest with no extra prefix
 * and return v,r,s in the same format as signTypedData.
 */
export type ClientEvmSigner = {
  readonly address: `0x${string}`;
  signTypedData(message: {
    domain: Record<string, unknown>;
    types: Record<string, unknown>;
    primaryType: string;
    message: Record<string, unknown>;
  }): Promise<`0x${string}`>;
  /**
   * Optional: sign a raw 32-byte EIP-712 digest (no 0x19\x01 prefix added).
   * Required for correct verification on chains that use a fixed DOMAIN_SEPARATOR
   * (e.g. gatelayer_testnet USDC). If not set, signTypedData is used.
   */
  signDigest?(digest: `0x${string}`): Promise<`0x${string}`>;
};

/**
 * FacilitatorEvmSigner - Used by x402 facilitators to verify and settle payments
 * This is typically a viem PublicClient + WalletClient combination that can
 * read contract state, verify signatures, write transactions, and wait for receipts
 *
 * Supports multiple addresses for load balancing, key rotation, and high availability
 */
export type FacilitatorEvmSigner = {
  /**
   * Get all addresses this facilitator can use for signing
   * Enables dynamic address selection for load balancing and key rotation
   */
  getAddresses(): readonly `0x${string}`[];

  readContract(args: {
    address: `0x${string}`;
    abi: readonly unknown[];
    functionName: string;
    args?: readonly unknown[];
  }): Promise<unknown>;
  verifyTypedData(args: {
    address: `0x${string}`;
    domain: Record<string, unknown>;
    types: Record<string, unknown>;
    primaryType: string;
    message: Record<string, unknown>;
    signature: `0x${string}`;
  }): Promise<boolean>;
  writeContract(args: {
    address: `0x${string}`;
    abi: readonly unknown[];
    functionName: string;
    args: readonly unknown[];
  }): Promise<`0x${string}`>;
  sendTransaction(args: { to: `0x${string}`; data: `0x${string}` }): Promise<`0x${string}`>;
  waitForTransactionReceipt(args: { hash: `0x${string}` }): Promise<{ status: string }>;
  getCode(args: { address: `0x${string}` }): Promise<`0x${string}` | undefined>;
};

/**
 * Converts a signer to a ClientEvmSigner
 *
 * @param signer - The signer to convert to a ClientEvmSigner
 * @returns The converted signer
 */
export function toClientEvmSigner(signer: ClientEvmSigner): ClientEvmSigner {
  return signer;
}

/**
 * Wraps a ClientEvmSigner (e.g. from viem privateKeyToAccount) and adds signDigest
 * so that gatelayer_testnet USDC signatures verify (uses chain's DOMAIN_SEPARATOR).
 *
 * If the account has a .sign(msg) that accepts { hash: Hex }, it will be used for signDigest.
 * Otherwise pass a custom signDigest implementation (e.g. using ethers wallet.signingKey.signDigest).
 *
 * @param account - Base signer with address and signTypedData
 * @param signDigestImpl - Optional: (digest) => signature. If not set and account has .sign(), uses account.sign({ hash: digest })
 */
export function withSignDigest(
  account: Pick<ClientEvmSigner, "address" | "signTypedData"> & {
    sign?(message: { hash?: `0x${string}` }): Promise<`0x${string}`>;
  },
  signDigestImpl?: (digest: `0x${string}`) => Promise<`0x${string}`>,
): ClientEvmSigner {
  const signDigest =
    signDigestImpl ??
    (account.sign
      ? (digest: `0x${string}`) => account.sign!({ hash: digest })
      : undefined);
  return {
    address: account.address,
    signTypedData: account.signTypedData.bind(account),
    ...(signDigest && { signDigest }),
  };
}

/**
 * Converts a viem client with single address to a FacilitatorEvmSigner
 * Wraps the single address in a getAddresses() function for compatibility
 *
 * @param client - The client to convert (must have 'address' property)
 * @returns FacilitatorEvmSigner with getAddresses() support
 */
export function toFacilitatorEvmSigner(
  client: Omit<FacilitatorEvmSigner, "getAddresses"> & { address: `0x${string}` },
): FacilitatorEvmSigner {
  return {
    ...client,
    getAddresses: () => [client.address],
  };
}
