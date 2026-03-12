import {
  decodePaymentRequiredHeader,
  decodePaymentResponseHeader,
  encodePaymentSignatureHeader,
} from ".";
import { SettleResponse } from "../types";
import { PaymentPayload, PaymentRequired } from "../types/payments";
import { x402Client } from "../client/x402Client";

/** HTTP 402 status code */
const STATUS_PAYMENT_REQUIRED = 402;

/** Max retries after 402 (one retry with payment, aligned with Go). */
const MAX_PAYMENT_RETRIES = 1;

/**
 * HTTP-specific client for handling x402 payment protocol over HTTP.
 *
 * Wraps a x402Client to provide HTTP-specific encoding/decoding functionality
 * for payment headers and responses. Supports fetchWithPayment / getWithPayment
 * so callers can hit a paid resource and automatically handle 402 (PAYMENT-REQUIRED).
 */
export class x402HTTPClient {
  /**
   * Creates a new x402HTTPClient instance.
   *
   * @param client - The underlying x402Client for payment logic
   */
  constructor(private readonly client: x402Client) {}

  /**
   * Encodes a payment payload into appropriate HTTP headers based on version.
   *
   * @param paymentPayload - The payment payload to encode
   * @returns HTTP headers containing the encoded payment signature
   */
  encodePaymentSignatureHeader(paymentPayload: PaymentPayload): Record<string, string> {
    switch (paymentPayload.x402Version) {
      case 2:
        return {
          "PAYMENT-SIGNATURE": encodePaymentSignatureHeader(paymentPayload),
        };
      case 1:
        return {
          "X-PAYMENT": encodePaymentSignatureHeader(paymentPayload),
        };
      default:
        throw new Error(
          `Unsupported x402 version: ${(paymentPayload as PaymentPayload).x402Version}`,
        );
    }
  }

  /**
   * Extracts payment required information from HTTP response.
   *
   * @param getHeader - Function to retrieve header value by name (case-insensitive)
   * @param body - Optional response body for v1 compatibility
   * @returns The payment required object
   */
  getPaymentRequiredResponse(
    getHeader: (name: string) => string | null | undefined,
    body?: unknown,
  ): PaymentRequired {
    // v2
    const paymentRequired = getHeader("PAYMENT-REQUIRED");
    if (paymentRequired) {
      return decodePaymentRequiredHeader(paymentRequired);
    }

    // v1
    if (
      body &&
      body instanceof Object &&
      "x402Version" in body &&
      (body as PaymentRequired).x402Version === 1
    ) {
      return body as PaymentRequired;
    }

    throw new Error("Invalid payment required response");
  }

  /**
   * Extracts payment settlement response from HTTP headers.
   *
   * @param getHeader - Function to retrieve header value by name (case-insensitive)
   * @returns The settlement response object
   */
  getPaymentSettleResponse(getHeader: (name: string) => string | null | undefined): SettleResponse {
    // v2
    const paymentResponse = getHeader("PAYMENT-RESPONSE");
    if (paymentResponse) {
      return decodePaymentResponseHeader(paymentResponse);
    }

    // v1
    const xPaymentResponse = getHeader("X-PAYMENT-RESPONSE");
    if (xPaymentResponse) {
      return decodePaymentResponseHeader(xPaymentResponse);
    }

    throw new Error("Payment response header not found");
  }

  /**
   * Creates a payment payload for the given payment requirements.
   * Delegates to the underlying x402Client.
   *
   * @param paymentRequired - The payment required response from the server
   * @returns Promise resolving to the payment payload
   */
  async createPaymentPayload(paymentRequired: PaymentRequired): Promise<PaymentPayload> {
    return this.client.createPaymentPayload(paymentRequired);
  }

  /**
   * Performs a fetch with automatic 402 handling: on 402, parses PAYMENT-REQUIRED (or V1 body),
   * creates a payment payload, and retries the request with PAYMENT-SIGNATURE / X-PAYMENT headers.
   * Aligned with Go WrapHTTPClientWithPayment + PaymentRoundTripper.
   *
   * @param input - URL string or Request (if Request, body is consumed on first fetch)
   * @param init - Optional fetch init (for Request input, use init to pass extra options)
   * @returns The final Response (after at most one retry with payment)
   */
  async fetchWithPayment(
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    let firstInit: RequestInit;
    let bodyForRetry: BodyInit | undefined;

    if (typeof input === "object" && input instanceof Request) {
      firstInit = {
        method: input.method,
        headers: input.headers,
        body: input.body,
        mode: input.mode,
        credentials: input.credentials,
        cache: input.cache,
        redirect: input.redirect,
        referrer: input.referrer,
        integrity: input.integrity,
      };
      const cloned = input.clone();
      bodyForRetry = await cloned.arrayBuffer();
    } else {
      firstInit = { ...init };
      bodyForRetry = firstInit.body ?? undefined;
    }

    const response = await fetch(url, firstInit);

    if (response.status !== STATUS_PAYMENT_REQUIRED) {
      return response;
    }

    const getHeader = (name: string): string | null => {
      const h = response.headers.get(name);
      if (h != null) return h;
      const lower = name.toLowerCase();
      for (const [k, v] of response.headers.entries()) {
        if (k.toLowerCase() === lower) return v;
      }
      return null;
    };

    let bodyJson: unknown = undefined;
    try {
      const text = await response.text();
      if (text) {
        bodyJson = JSON.parse(text) as unknown;
      }
    } catch {
      // ignore body parse
    }

    let paymentRequired: PaymentRequired;
    try {
      paymentRequired = this.getPaymentRequiredResponse(getHeader, bodyJson);
    } catch (e) {
      throw new Error(
        `Failed to parse 402 Payment Required: ${e instanceof Error ? e.message : String(e)}`,
      );
    }

    const paymentPayload = await this.createPaymentPayload(paymentRequired);
    const paymentHeaders = this.encodePaymentSignatureHeader(paymentPayload);

    const nextHeaders = new Headers(firstInit.headers ?? {});
    for (const [k, v] of Object.entries(paymentHeaders)) {
      nextHeaders.set(k, v);
    }

    const secondResponse = await fetch(url, {
      ...firstInit,
      headers: nextHeaders,
      body: bodyForRetry ?? undefined,
    });

    if (secondResponse.status === STATUS_PAYMENT_REQUIRED) {
      throw new Error(
        "Payment retry limit exceeded: server returned 402 again after sending payment",
      );
    }

    return secondResponse;
  }

  /**
   * GET with automatic 402 handling. Convenience for fetchWithPayment(url, { method: "GET" }).
   *
   * @param url - Resource URL
   * @param init - Optional fetch init
   * @returns The final Response
   */
  async getWithPayment(url: string, init?: RequestInit): Promise<Response> {
    return this.fetchWithPayment(url, { ...init, method: "GET" });
  }

  /**
   * POST with automatic 402 handling. Convenience for fetchWithPayment(url, { method: "POST", body }).
   *
   * @param url - Resource URL
   * @param body - Optional body (e.g. JSON string or FormData)
   * @param init - Optional fetch init
   * @returns The final Response
   */
  async postWithPayment(
    url: string,
    body?: BodyInit | null,
    init?: RequestInit,
  ): Promise<Response> {
    return this.fetchWithPayment(url, { ...init, method: "POST", body: body ?? undefined });
  }
}
