import { PaymentPayload, PaymentRequirements } from "../types/payments";
import {
  VerifyResponse,
  SettleResponse,
  SupportedResponse,
  SupportedKind,
  VerifyError,
  SettleError,
} from "../types/facilitator";

/** Default facilitator URL (Gate Web3 OpenAPI Testnet). Aligned with Go DefaultFacilitatorURL. */
export const DEFAULT_FACILITATOR_URL = "https://openapi-test.gateweb3.cc/api/v1/x402";

const GATE_WEB3_SIGNING_PATH = "/api/v1/x402";
const TARGET_URI_SUPPORTED = "v1/x402/supported";
const TARGET_URI_VERIFY = "v1/x402/verify";
const TARGET_URI_SETTLE = "v1/x402/settle";

const ENV_GATE_WEB3_API_KEY = "GATE_WEB3_API_KEY";
const ENV_GATE_WEB3_API_SECRET = "GATE_WEB3_API_SECRET";
const ENV_GATE_WEB3_PASSPHRASE = "GATE_WEB3_PASSPHRASE";
const ENV_GATE_WEB3_REAL_IP = "GATE_WEB3_REAL_IP";
const DEFAULT_GATE_WEB3_FORWARDED_FOR = "127.0.0.1";

export interface FacilitatorConfig {
  /** Base URL of the facilitator. If empty, uses DEFAULT_FACILITATOR_URL. */
  url?: string;
  /** Optional custom auth headers (overrides Gate Web3 env-based signing when provided). */
  createAuthHeaders?: () => Promise<{
    verify: Record<string, string>;
    settle: Record<string, string>;
    supported: Record<string, string>;
  }>;
}

/**
 * Interface for facilitator clients (aligned with Go FacilitatorClient).
 */
export interface FacilitatorClient {
  verify(
    paymentPayload: PaymentPayload,
    paymentRequirements: PaymentRequirements,
  ): Promise<VerifyResponse>;

  settle(
    paymentPayload: PaymentPayload,
    paymentRequirements: PaymentRequirements,
  ): Promise<SettleResponse>;

  getSupported(): Promise<SupportedResponse>;
}

/** API envelope: { code, msg, data } used by Gate OpenAPI. */
interface FacilitatorApiResponse<T> {
  code: number;
  msg: string;
  data: T;
}

/**
 * Build Gate Web3 HMAC-SHA256 auth headers.
 * PREHASH = <timestamp><GATE_WEB3_SIGNING_PATH><rawBody>
 * Signature = Base64(HMAC_SHA256(SK, PREHASH))
 * Uses env: GATE_WEB3_API_KEY, GATE_WEB3_API_SECRET, GATE_WEB3_PASSPHRASE, GATE_WEB3_REAL_IP.
 * Returns {} when not in Node or when AK/SK are missing.
 * Uses dynamic `import("node:crypto")` so Gate signing works under ESM (e.g. `tsx`); `require` is undefined there and would silently skip auth.
 */
async function buildGateWeb3Headers(
  body: string,
  targetUri: string,
): Promise<Record<string, string>> {
  const getEnv = (key: string): string => {
    if (typeof process === "undefined" || !process.env) return "";
    const v = process.env[key];
    return typeof v === "string" ? v.trim() : "";
  };
  const ak = getEnv(ENV_GATE_WEB3_API_KEY);
  const sk = getEnv(ENV_GATE_WEB3_API_SECRET);
  if (!ak || !sk) return {};

  let crypto: typeof import("node:crypto");
  try {
    crypto = await import("node:crypto");
  } catch {
    return {};
  }

  const timestamp = Date.now();
  const prehash = `${timestamp}${GATE_WEB3_SIGNING_PATH}${body}`;
  const sig = crypto.createHmac("sha256", sk).update(prehash, "utf8").digest("base64");

  const pass = getEnv(ENV_GATE_WEB3_PASSPHRASE) || "";
  const realIp = getEnv(ENV_GATE_WEB3_REAL_IP) || DEFAULT_GATE_WEB3_FORWARDED_FOR;

  const headers: Record<string, string> = {
    "X-Api-Key": ak,
    "X-Timestamp": String(timestamp),
    "X-Signature": sig,
    "X-Request-Id": crypto.randomUUID?.() ?? `req-${timestamp}-${Math.random().toString(36).slice(2)}`,
    "x-target-uri": targetUri.startsWith("/") ? targetUri.slice(1) : targetUri,
  };
  if (pass) headers["X-Passphrase"] = pass;
  if (realIp) headers["X-Forwarded-For"] = realIp;

  return headers;
}

/**
 * HTTP client for x402 facilitator services.
 * Aligned with Go HTTPFacilitatorClient: single POST endpoint, action/params envelope, code/msg/data response, Gate Web3 signing.
 */
export class HTTPFacilitatorClient implements FacilitatorClient {
  readonly url: string;
  private readonly _createAuthHeaders?: FacilitatorConfig["createAuthHeaders"];

  constructor(config?: FacilitatorConfig) {
    this.url = (config?.url ?? "").trim() || DEFAULT_FACILITATOR_URL;
    this._createAuthHeaders = config?.createAuthHeaders;
  }

  private toJsonSafe(obj: unknown): unknown {
    return JSON.parse(
      JSON.stringify(obj, (_, value) => (typeof value === "bigint" ? value.toString() : value)),
    );
  }

  private async authHeadersFor(
    endpoint: "verify" | "settle" | "supported",
    body: string,
    targetUri: string,
  ): Promise<Record<string, string>> {
    const gateHeaders = await buildGateWeb3Headers(body, targetUri);
    if (this._createAuthHeaders) {
      const custom = await this._createAuthHeaders();
      const customForEndpoint = custom[endpoint] ?? {};
      return { ...gateHeaders, ...customForEndpoint };
    }
    return gateHeaders;
  }

  /**
   * POST one action to the facilitator and return parsed envelope (code, msg, data).
   */
  private async postEnvelope<T>(
    action: string,
    params: Record<string, unknown>,
    targetUri: string,
  ): Promise<{ status: number; code: number; msg: string; data: T }> {
    const body = JSON.stringify({ action, params });
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...(await this.authHeadersFor(
        action === "x402.supported" ? "supported" : action === "x402.verify" ? "verify" : "settle",
        body,
        targetUri,
      )),
    };

    const response = await fetch(this.url, {
      method: "POST",
      headers,
      body,
    });

    const rawText = await response.text();

    // Gate OpenAPI typically returns { code, msg, data }, but be tolerant to:
    // - msg vs message
    // - data vs result
    // - numeric fields encoded as strings
    let parsed: unknown;
    try {
      parsed = rawText ? (JSON.parse(rawText) as unknown) : {};
    } catch {
      return {
        status: response.status,
        code: -1,
        msg: `non-json facilitator response: ${rawText.slice(0, 300)}`,
        data: {} as T,
      };
    }

    const raw = parsed as Partial<
      FacilitatorApiResponse<T> & { message?: string; result?: T; status?: number | string }
    >;

    const code =
      typeof raw.code === "number"
        ? raw.code
        : typeof raw.code === "string"
          ? Number(raw.code)
          : typeof raw.status === "number"
            ? raw.status
            : typeof raw.status === "string"
              ? Number(raw.status)
              : -1;

    let msg =
      typeof raw.msg === "string"
        ? raw.msg
        : typeof raw.message === "string"
          ? raw.message
          : "";

    const data = ((raw.data ?? raw.result) ?? {}) as T;

    // Some Gate OpenAPI errors return a business `code` with empty msg/data.
    // Keep the client debuggable by surfacing the raw response (truncated).
    if ((!msg || msg.trim() === "") && code !== 0) {
      msg = `raw=${rawText.slice(0, 300)}`;
    }

    return { status: response.status, code, msg, data };
  }

  async verify(
    paymentPayload: PaymentPayload,
    paymentRequirements: PaymentRequirements,
  ): Promise<VerifyResponse> {
    const params = {
      x402Version: paymentPayload.x402Version,
      paymentPayload: this.toJsonSafe(paymentPayload) as Record<string, unknown>,
      paymentRequirements: this.toJsonSafe(paymentRequirements) as Record<string, unknown>,
    };

    const { status, code, msg, data } = await this.postEnvelope<VerifyResponse>(
      "x402.verify",
      params,
      TARGET_URI_VERIFY,
    );

    if (status !== 200 || code !== 0) {
      // Include Gate business error code/msg if invalidReason is not provided
      const codeHint = code && code !== 0 ? `code=${code}` : "code=?";
      const msgHint = msg ? ` msg=${msg}` : "";
      const resp: VerifyResponse = {
        isValid: false,
        invalidReason:
          data.invalidReason ||
          msg ||
          `facilitator verify failed (http=${status}, ${codeHint}${msgHint})`,
        payer: data.payer,
      };
      throw new VerifyError(status, resp);
    }

    return data;
  }

  async settle(
    paymentPayload: PaymentPayload,
    paymentRequirements: PaymentRequirements,
  ): Promise<SettleResponse> {
    const params = {
      x402Version: paymentPayload.x402Version,
      paymentPayload: this.toJsonSafe(paymentPayload) as Record<string, unknown>,
      paymentRequirements: this.toJsonSafe(paymentRequirements) as Record<string, unknown>,
    };

    const { status, code, msg, data } = await this.postEnvelope<SettleResponse>(
      "x402.settle",
      params,
      TARGET_URI_SETTLE,
    );

    if (status !== 200 || code !== 0) {
      const resp: SettleResponse = {
        success: false,
        errorReason:
          (data.errorReason ?? msg) ||
          `facilitator settle failed (http=${status}, code=${code || -1})`,
        payer: data.payer,
        transaction: data.transaction ?? "",
        network: (data.network ?? "eip155:0") as import("../types").Network,
      };
      throw new SettleError(status, resp);
    }

    return data;
  }

  async getSupported(): Promise<SupportedResponse> {
    const { status, code, msg, data } = await this.postEnvelope<SupportedResponse>(
      "x402.supported",
      {},
      TARGET_URI_SUPPORTED,
    );

    if (status !== 200 || code !== 0) {
      throw new Error(`Facilitator getSupported failed (http=${status}, code=${code}, msg=${msg})`);
    }

    return {
      kinds: data.kinds ?? [],
      extensions: data.extensions ?? [],
      signers: data.signers ?? {},
    };
  }

  /**
   * Optional: create auth headers for a given path (for custom middleware).
   */
  async createAuthHeaders(path: string): Promise<{ headers: Record<string, string> }> {
    const body = JSON.stringify({ action: `x402.${path}`, params: {} });
    const targetUri =
      path === "supported"
        ? TARGET_URI_SUPPORTED
        : path === "verify"
          ? TARGET_URI_VERIFY
          : TARGET_URI_SETTLE;
    const headers = await this.authHeadersFor(
      path === "supported" ? "supported" : path === "verify" ? "verify" : "settle",
      body,
      targetUri,
    );
    return { headers };
  }
}
