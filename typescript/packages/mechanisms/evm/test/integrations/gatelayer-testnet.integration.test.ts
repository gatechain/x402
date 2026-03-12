import { describe, it, expect } from "vitest";
import { x402Client, x402HTTPClient } from "@x402/core/client";
import {
  HTTPAdapter,
  HTTPResponseInstructions,
  x402HTTPResourceServer,
  x402ResourceServer,
} from "@x402/core/server";
import { HTTPFacilitatorClient } from "@x402/core/http";
import {
  Network,
  PaymentPayload,
  PaymentRequirements,
} from "@x402/core/types";
import { ExactEvmScheme as ExactEvmClient, createSignerFromPrivateKey } from "../../src";
import { ExactEvmScheme as ExactEvmServer } from "../../src/exact/server/scheme";

// 环境变量（与 Go 集成测试保持一致）
const PAYEE_ADDRESS = process.env.PAYEE_ADDRESS as `0x${string}` | undefined;
const EVM_PRIVATE_KEY = process.env.EVM_PRIVATE_KEY as `0x${string}` | undefined;
const GATE_WEB3_API_KEY = process.env.GATE_WEB3_API_KEY;
const GATE_WEB3_API_SECRET = process.env.GATE_WEB3_API_SECRET;

const hasEnvForGateFlow =
  !!PAYEE_ADDRESS && !!EVM_PRIVATE_KEY && !!GATE_WEB3_API_KEY && !!GATE_WEB3_API_SECRET;

// 简单的 HTTP 适配器，用于驱动 x402HTTPResourceServer（无需真实 Node HTTP 框架）
const makeMockAdapter = (
  path: string,
  url: string,
  headers: Record<string, string | undefined> = {},
): HTTPAdapter => ({
  getHeader: (name: string) => headers[name],
  getMethod: () => "GET",
  getPath: () => path,
  getUrl: () => url,
  getAcceptHeader: () => "application/json",
  getUserAgent: () => "TestClient/1.0",
});

// 只在环境变量齐全时真正跑 Gate gatelayer_testnet 的端到端流程，否则跳过
const gateIt = hasEnvForGateFlow ? it : it.skip;

describe("Gate gatelayer_testnet end-to-end (TypeScript)", () => {
  gateIt(
    "should perform full payment flow via Gate Web3 facilitator using gatelayer_testnet and private key signer",
    async () => {
      // ---------- Server 侧：构建资源服务器 + HTTP 适配 ----------
      const network = "gatelayer_testnet" as Network;

      // 使用真实 Gate Web3 OpenAPI 的 HTTPFacilitatorClient
      const facilitatorClient = new HTTPFacilitatorClient();

      const resourceServer = new x402ResourceServer(facilitatorClient);
      resourceServer.register(network, new ExactEvmServer());
      await resourceServer.initialize();

      const routes = {
        "/weather": {
          accepts: {
            scheme: "exact",
            payTo: PAYEE_ADDRESS!,
            price: "$0.001", // 与 Go 测试一致：按 USD 计价，由 facilitator 映射到默认 USDC
            network,
          },
          description: "Gate weather demo (TypeScript)",
          mimeType: "application/json",
        },
      };

      const httpServer = new x402HTTPResourceServer(resourceServer, routes);

      const adapterPath = "/weather";
      const adapterUrl = "https://example.com/weather";

      // 第一次请求：未携带支付信息，应返回 402 + PAYMENT-REQUIRED
      const context1 = {
        adapter: makeMockAdapter(adapterPath, adapterUrl),
        path: adapterPath,
        method: "GET",
      };

      const httpProcessResult1 = await httpServer.processHTTPRequest(context1);

      expect(httpProcessResult1.type).toBe("payment-error");

      const initial402Response = (
        httpProcessResult1 as { type: "payment-error"; response: HTTPResponseInstructions }
      ).response;

      expect(initial402Response.status).toBe(402);
      expect(initial402Response.headers["PAYMENT-REQUIRED"]).toBeDefined();

      // ---------- Client 侧：使用私钥 signer + ExactEvmScheme + x402HTTPClient ----------
      const signer = createSignerFromPrivateKey(EVM_PRIVATE_KEY!);
      const coreClient = new x402Client().register(network, new ExactEvmClient(signer));
      const httpClient = new x402HTTPClient(coreClient);

      // 解析 402 的 PAYMENT-REQUIRED，生成支付 payload
      const paymentRequired = httpClient.getPaymentRequiredResponse(
        name => initial402Response.headers[name],
        initial402Response.body,
      );

      const paymentPayload = await httpClient.createPaymentPayload(paymentRequired);

      expect(paymentPayload).toBeDefined();
      expect(paymentPayload.accepted.scheme).toBe("exact");
      expect(String(paymentPayload.accepted.network)).toBe("gatelayer_testnet");

      // 将支付签名编码为 HTTP 头
      const paymentHeaders = httpClient.encodePaymentSignatureHeader(paymentPayload);
      expect(paymentHeaders["PAYMENT-SIGNATURE"] ?? paymentHeaders["X-PAYMENT"]).toBeDefined();

      // 第二次请求：携带 PAYMENT-SIGNATURE 头，服务端应验证签名并结算成功
      const context2Headers: Record<string, string | undefined> = {
        "PAYMENT-SIGNATURE": paymentHeaders["PAYMENT-SIGNATURE"],
      };

      const context2 = {
        adapter: makeMockAdapter(adapterPath, adapterUrl, context2Headers),
        path: adapterPath,
        method: "GET",
      };

      const httpProcessResult2 = await httpServer.processHTTPRequest(context2);

      expect(httpProcessResult2.type).toBe("payment-verified");

      const {
        paymentPayload: verifiedPaymentPayload,
        paymentRequirements: verifiedPaymentRequirements,
      } = httpProcessResult2 as {
        type: "payment-verified";
        paymentPayload: PaymentPayload;
        paymentRequirements: PaymentRequirements;
      };

      expect(verifiedPaymentPayload).toBeDefined();
      expect(verifiedPaymentRequirements).toBeDefined();

      // 结算，要求 facilitator 在 Gate 上完成处理。这里会真正调用 Gate Web3 OpenAPI。
      const settleResult = await httpServer.processSettlement(
        verifiedPaymentPayload,
        verifiedPaymentRequirements,
        200,
      );

      expect(settleResult).toBeDefined();
      expect(settleResult.success).toBe(true);
      expect(settleResult.network).toBe(network);
      expect(settleResult.payer).toBeDefined();
      expect(settleResult.transaction).toBeDefined();
    },
  );
});

