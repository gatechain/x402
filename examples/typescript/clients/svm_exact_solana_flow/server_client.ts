import express from "express";
import { paymentMiddleware, x402ResourceServer } from "@x402/express";
import { HTTPFacilitatorClient } from "@x402/core/server";
import { x402Client } from "@x402/core/client";
import { ExactSvmScheme as ExactSvmServerScheme } from "@x402/svm/exact/server";
import { ExactSvmScheme as ExactSvmClientScheme } from "@x402/svm/exact/client";
import {
  decodePaymentRequiredHeader,
  decodePaymentResponseHeader,
  encodePaymentSignatureHeader,
} from "@x402/core/http";
import type { PaymentRequired, PaymentRequirements } from "@x402/core/types";
import { createKeyPairSignerFromBytes } from "@solana/kit";
import { base58 } from "@scure/base";

const PORT = Number(process.env.PORT ?? 4025);
const ROUTE_PATH = process.env.ROUTE_PATH ?? "/pay";
const SERVER_URL = `http://localhost:${PORT}${ROUTE_PATH}`;

const DEFAULT_FACILITATOR_URL = "https://openapi-test.gateweb3.cc/api/v1/x402";
const FACILITATOR_URL = (process.env.FACILITATOR_URL ?? "").trim() || DEFAULT_FACILITATOR_URL;

// Gate openapi-test uses V1 Solana network names in /supported and verify.
const SVM_NETWORK = (process.env.SVM_NETWORK ?? "solana-devnet").trim();

// OpenAPI-test devnet asset mint you provided
const DEFAULT_MINT = "BPy1fp1Hb1v6Rr41ayPs8ttRUrjjNqkApudTiinNucg3";
const SVM_ASSET_MINT = (process.env.SVM_ASSET_MINT ?? DEFAULT_MINT).trim();

// 6 decimals confirmed by you (100000 == 0.10)
const PAYMENT_AMOUNT_ATOMIC = (process.env.PAYMENT_AMOUNT_ATOMIC ?? "100000").trim();

const SVM_PAYEE_ADDRESS = (process.env.SVM_PAYEE_ADDRESS ?? "").trim();
if (!SVM_PAYEE_ADDRESS) {
  console.error("SVM_PAYEE_ADDRESS required (base58)");
  process.exit(1);
}

// Required workaround: openapi-test /supported doesn't provide feePayer/signers.
const SVM_FEE_PAYER = (process.env.SVM_FEE_PAYER ?? "").trim();
if (!SVM_FEE_PAYER) {
  console.error("SVM_FEE_PAYER required (base58 fee payer address managed by facilitator)");
  process.exit(1);
}

const SVM_CLIENT_PRIVATE_KEY = (process.env.SVM_CLIENT_PRIVATE_KEY ?? "").trim();
if (!SVM_CLIENT_PRIVATE_KEY) {
  console.error("SVM_CLIENT_PRIVATE_KEY required (base58 bytes)");
  process.exit(1);
}

function requireGateOpenapiCredentials(): void {
  const ak = (process.env.GATE_WEB3_API_KEY ?? "").trim();
  const sk = (process.env.GATE_WEB3_API_SECRET ?? "").trim();
  if (ak && sk) return;
  console.error("GATE_WEB3_API_KEY and GATE_WEB3_API_SECRET are required for openapi-test facilitator.");
  process.exit(1);
}

async function runClient(): Promise<void> {
  const initial = await fetch(SERVER_URL, { method: "GET" });
  console.log(`TS(SVM) client initial status=${initial.status}`);

  if (initial.status !== 402) {
    console.log("Initial body:", await initial.text().catch(() => ""));
    return;
  }

  const paymentRequiredHeader = initial.headers.get("PAYMENT-REQUIRED");
  if (!paymentRequiredHeader) throw new Error("Missing PAYMENT-REQUIRED");

  const paymentRequired = decodePaymentRequiredHeader(paymentRequiredHeader) as PaymentRequired;
  console.log("Initial PAYMENT-REQUIRED accepts[0].extra:", paymentRequired.accepts?.[0]?.extra ?? {});
  const originalAccepts = paymentRequired.accepts;

  // Gate openapi-test currently does not provide feePayer/signers in /supported,
  // so the server may not be able to inject feePayer into accepts[].extra.
  // Make the demo runnable by injecting feePayer from env before building the client payload.
  const acceptsForSigning = paymentRequired.accepts.map(acc => {
    if (acc.scheme === "exact" && String(acc.network) === SVM_NETWORK) {
      return {
        ...acc,
        extra: {
          ...(acc.extra ?? {}),
          feePayer: (acc.extra as Record<string, unknown> | undefined)?.feePayer ?? SVM_FEE_PAYER,
        } as Record<string, unknown>,
      };
    }
    return acc;
  });
  const paymentRequiredForSigning: PaymentRequired = {
    ...paymentRequired,
    accepts: acceptsForSigning,
  };

  const selected = paymentRequiredForSigning.accepts.find(
    a => a.scheme === "exact" && String(a.network) === SVM_NETWORK,
  ) as PaymentRequirements | undefined;
  if (!selected) {
    throw new Error(
      `No accepted requirement exact+${SVM_NETWORK}. accepts=${JSON.stringify(paymentRequiredForSigning.accepts)}`,
    );
  }

  console.log("Selected requirements extra:", selected.extra);

  const clientKeypair = await createKeyPairSignerFromBytes(base58.decode(SVM_CLIENT_PRIVATE_KEY));

  const client = new x402Client().register(SVM_NETWORK as never, new ExactSvmClientScheme(clientKeypair));

  const payload = await client.createPaymentPayload(paymentRequiredForSigning);
  // Keep accepted exactly as the server originally advertised, so server-side requirement matching succeeds.
  const originalSelected = originalAccepts.find(
    a => a.scheme === "exact" && String(a.network) === SVM_NETWORK,
  ) as PaymentRequirements | undefined;
  if (originalSelected) {
    payload.accepted = originalSelected;
  }
  const paymentHeader = encodePaymentSignatureHeader(payload);

  console.log("TS(SVM) client retry with PAYMENT-SIGNATURE...");
  const retry = await fetch(SERVER_URL, {
    method: "GET",
    headers: { "PAYMENT-SIGNATURE": paymentHeader },
  });

  console.log(`TS(SVM) retry status=${retry.status}`);
  const retryText = await retry.text().catch(() => "");
  console.log("TS(SVM) retry body:", retryText);

  if (retry.status === 402) {
    const again = retry.headers.get("PAYMENT-REQUIRED");
    if (again) {
      try {
        const pr = decodePaymentRequiredHeader(again) as PaymentRequired;
        console.log("TS(SVM) retry PAYMENT-REQUIRED error:", pr.error ?? "(none)");
      } catch {
        console.log("TS(SVM) retry PAYMENT-REQUIRED: (decode failed)");
      }
    }
  }

  const paymentResponseHeader = retry.headers.get("PAYMENT-RESPONSE");
  if (paymentResponseHeader) {
    const paymentResponse = decodePaymentResponseHeader(paymentResponseHeader);
    console.log("PAYMENT-RESPONSE:", paymentResponse);
  }
}

async function main(): Promise<void> {
  requireGateOpenapiCredentials();

  const facilitatorClient = new HTTPFacilitatorClient({ url: FACILITATOR_URL });
  const resourceServer = new x402ResourceServer(facilitatorClient).register(
    SVM_NETWORK as never,
    new ExactSvmServerScheme(),
  );

  // Workaround for openapi-test: /supported does not provide feePayer/signers.
  // Keep server-advertised requirements unchanged for matching, but inject feePayer right before verify/settle.
  resourceServer.onBeforeVerify(async ({ requirements }) => {
    requirements.extra = requirements.extra ?? {};
    const extra = requirements.extra as Record<string, unknown>;
    if (!extra.feePayer) extra.feePayer = SVM_FEE_PAYER;
  });
  resourceServer.onBeforeSettle(async ({ requirements }) => {
    requirements.extra = requirements.extra ?? {};
    const extra = requirements.extra as Record<string, unknown>;
    if (!extra.feePayer) extra.feePayer = SVM_FEE_PAYER;
  });

  const app = express();

  app.use(
    paymentMiddleware(
      {
        [`GET ${ROUTE_PATH}`]: {
          accepts: [
            {
              scheme: "exact",
              network: SVM_NETWORK,
              payTo: SVM_PAYEE_ADDRESS,
              price: {
                amount: PAYMENT_AMOUNT_ATOMIC,
                asset: SVM_ASSET_MINT,
                extra: {
                  feePayer: SVM_FEE_PAYER,
                },
              },
              maxTimeoutSeconds: 60,
            },
          ],
          description: "x402 exact payment (Solana devnet)",
          mimeType: "application/json",
        },
      },
      resourceServer,
    ),
  );

  app.get(ROUTE_PATH, (_req, res) => {
    res.json({
      ok: true,
      route: ROUTE_PATH,
      network: SVM_NETWORK,
      asset: SVM_ASSET_MINT,
      amount: PAYMENT_AMOUNT_ATOMIC,
    });
  });

  const server = app.listen(PORT, async () => {
    console.log(`TS(SVM) x402 server listening ${SERVER_URL}`);
    try {
      await runClient();
    } finally {
      if (process.env.KEEP_ALIVE !== "1") server.close();
    }
  });
}

main().catch(e => {
  console.error(e);
  process.exit(1);
});

