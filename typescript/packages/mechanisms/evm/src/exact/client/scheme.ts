import { PaymentPayload, PaymentRequirements, SchemeNetworkClient } from "@x402/core/types";
import { getAddress } from "viem";
import { authorizationTypes } from "../../constants";
import {
  buildEip712DigestTransferWithAuthorization,
  GATELAYER_TESTNET_USDC_ADDRESS,
  GATELAYER_TESTNET_USDC_DOMAIN_SEPARATOR,
} from "../../gatelayer";
import { ClientEvmSigner } from "../../signer";
import { ExactEvmPayloadV2 } from "../../types";
import { createNonce, getEvmChainIdFromNetwork } from "../../utils";

/**
 * EVM client implementation for the Exact payment scheme.
 *
 */
export class ExactEvmScheme implements SchemeNetworkClient {
  readonly scheme = "exact";

  /**
   * Creates a new ExactEvmClient instance.
   *
   * @param signer - The EVM signer for client operations
   */
  constructor(private readonly signer: ClientEvmSigner) {}

  /**
   * Creates a payment payload for the Exact scheme.
   *
   * @param x402Version - The x402 protocol version
   * @param paymentRequirements - The payment requirements
   * @returns Promise resolving to a payment payload
   */
  async createPaymentPayload(
    x402Version: number,
    paymentRequirements: PaymentRequirements,
  ): Promise<Pick<PaymentPayload, "x402Version" | "payload">> {
    const nonce = createNonce();
    const now = Math.floor(Date.now() / 1000);

    const authorization: ExactEvmPayloadV2["authorization"] = {
      from: this.signer.address,
      to: getAddress(paymentRequirements.payTo),
      value: paymentRequirements.amount,
      validAfter: (now - 600).toString(), // 10 minutes before
      validBefore: (now + paymentRequirements.maxTimeoutSeconds).toString(),
      nonce,
    };

    // Sign the authorization
    const signature = await this.signAuthorization(authorization, paymentRequirements);

    const payload: ExactEvmPayloadV2 = {
      authorization,
      signature,
    };

    return {
      x402Version,
      payload,
    };
  }

  /**
   * Sign the EIP-3009 authorization using EIP-712.
   * For gatelayer_testnet + Gate USDC, uses chain's DOMAIN_SEPARATOR when signDigest is provided.
   *
   * @param authorization - The authorization to sign
   * @param requirements - The payment requirements
   * @returns Promise resolving to the signature
   */
  private async signAuthorization(
    authorization: ExactEvmPayloadV2["authorization"],
    requirements: PaymentRequirements,
  ): Promise<`0x${string}`> {
    const assetLower = requirements.asset?.toLowerCase() ?? "";
    const useGatelayerSeparator =
      String(requirements.network) === "gatelayer_testnet" &&
      assetLower === GATELAYER_TESTNET_USDC_ADDRESS;

    if (useGatelayerSeparator) {
      if (typeof this.signer.signDigest !== "function") {
        throw new Error(
          "For gatelayer_testnet with USDC at 0x9be8Df37C788B244cFc28E46654aD5Ec28a880AF, provide a signer with signDigest(digest) so the signature matches the chain's DOMAIN_SEPARATOR. Example: signDigest: (digest) => account.sign({ hash: digest })",
        );
      }
      const digest = buildEip712DigestTransferWithAuthorization(
        GATELAYER_TESTNET_USDC_DOMAIN_SEPARATOR,
        {
          from: authorization.from,
          to: authorization.to,
          value: authorization.value,
          validAfter: authorization.validAfter,
          validBefore: authorization.validBefore,
          nonce: authorization.nonce as `0x${string}`,
        },
      );
      return this.signer.signDigest(digest);
    }

    const chainId = getEvmChainIdFromNetwork(requirements.network);

    if (!requirements.extra?.name || !requirements.extra?.version) {
      throw new Error(
        `EIP-712 domain parameters (name, version) are required in payment requirements for asset ${requirements.asset}`,
      );
    }

    const { name, version } = requirements.extra;

    const domain = {
      name,
      version,
      chainId,
      verifyingContract: getAddress(requirements.asset),
    };

    const message = {
      from: getAddress(authorization.from),
      to: getAddress(authorization.to),
      value: BigInt(authorization.value),
      validAfter: BigInt(authorization.validAfter),
      validBefore: BigInt(authorization.validBefore),
      nonce: authorization.nonce,
    };

    return await this.signer.signTypedData({
      domain,
      types: authorizationTypes,
      primaryType: "TransferWithAuthorization",
      message,
    });
  }
}
