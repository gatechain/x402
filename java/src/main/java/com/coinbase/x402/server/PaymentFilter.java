package com.coinbase.x402.server;

import com.coinbase.x402.client.FacilitatorClient;
import com.coinbase.x402.client.SettlementResponse;
import com.coinbase.x402.client.VerificationResponse;
import com.coinbase.x402.model.ExactSchemePayload;
import com.coinbase.x402.model.PaymentPayload;
import com.coinbase.x402.model.PaymentRequirements;
import com.coinbase.x402.model.PaymentRequiredResponse;
import com.coinbase.x402.model.SettlementResponseHeader;
import com.coinbase.x402.model.PaymentPayloadV2;
import com.coinbase.x402.model.PaymentRequirementsV2;
import com.coinbase.x402.model.PaymentRequiredV2;
import com.coinbase.x402.model.ResourceInfo;
import com.coinbase.x402.model.SettlementResponseV2;
import com.coinbase.x402.util.Json;

import jakarta.servlet.Filter;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.ServletRequest;
import jakarta.servlet.ServletResponse;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.math.BigInteger;
import java.util.Map;
import java.util.Objects;

/** Servlet/Spring filter that enforces x402 payments on selected paths. */
public class PaymentFilter implements Filter {

    private final String                       payTo;
    private final Map<String, BigInteger>      priceTable;   // path → amount
    private final FacilitatorClient            facilitator;

    /**
     * Creates a payment filter that enforces X-402 payments on configured paths.
     *
     * @param payTo wallet address for payments
     * @param priceTable maps request paths to required payment amounts in atomic units.
     *                   Uses exact, case-sensitive matching against {@code HttpServletRequest#getRequestURI()}.
     *                   Query parameters are included in matching, HTTP method is ignored.
     *                   Paths not present in the map allow free access. Values are atomic units
     *                   assuming 6-decimal tokens (10000 = 0.01 USDC, 1000000 = 1.00 USDC).
     * @param facilitator client for payment verification and settlement
     * @apiNote
     * <p><strong>Path matching</strong></p>
     * <ul>
     *   <li>Exact, case-sensitive compare of {@code HttpServletRequest#getRequestURI()}</li>
     *   <li>Query string included; HTTP method ignored</li>
     *   <li>URIs not present in the map are free</li>
     * </ul>
     *
     * <p><strong>Price units</strong> — amounts assume a 6-decimal token (e.g. USDC).
     * Multiply by 10<sup>12</sup> for 18-decimal tokens.</p>
     *
     * <p><strong>Examples</strong></p>
     * <pre>{@code
     * Map<String, BigInteger> priceTable = Map.of(
     *     "/api/premium", BigInteger.valueOf( 10000),   // 0.01 USDC
     *     "/api/report",  BigInteger.valueOf(1000000)   // 1.00 USDC
     * );
     * }</pre>
     */
    public PaymentFilter(String payTo,
                         Map<String, BigInteger> priceTable,
                         FacilitatorClient facilitator) {
        this.payTo       = Objects.requireNonNull(payTo);
        this.priceTable  = Objects.requireNonNull(priceTable);
        this.facilitator = Objects.requireNonNull(facilitator);
    }

    /* ------------------------------------------------ core -------------- */

    @Override
    public void doFilter(ServletRequest req,
                         ServletResponse res,
                         FilterChain     chain)
            throws IOException, ServletException {

        if (!(req instanceof HttpServletRequest) ||
            !(res instanceof HttpServletResponse)) {
            chain.doFilter(req, res);          // non-HTTP
            return;
        }

        HttpServletRequest  request  = (HttpServletRequest)  req;
        HttpServletResponse response = (HttpServletResponse) res;
        String              path     = request.getRequestURI();

        /* -------- path is free? skip check ----------------------------- */
        if (!priceTable.containsKey(path)) {
            chain.doFilter(req, res);
            return;
        }

        String paymentSignatureHeader = request.getHeader("PAYMENT-SIGNATURE");
        String legacyHeader           = request.getHeader("X-PAYMENT");

        boolean hasV2Header = paymentSignatureHeader != null && !paymentSignatureHeader.isEmpty();
        boolean hasV1Header = legacyHeader != null && !legacyHeader.isEmpty();

        if (!hasV2Header && !hasV1Header) {
            respond402(response, path, null);
            return;
        }

        VerificationResponse vr;
        PaymentPayload       payloadV1 = null;
        PaymentPayloadV2     payloadV2 = null;
        String               headerForFacilitator;
        try {
            if (hasV2Header) {
                payloadV2 = PaymentPayloadV2.fromHeader(paymentSignatureHeader);
                headerForFacilitator = paymentSignatureHeader;

                // 对 V2：仍然要求 payload 中的 resource 字段与 path 匹配（与旧逻辑保持一致）
                Object res = payloadV2.payload != null ? payloadV2.payload.get("resource") : null;
                if (!Objects.equals(res, path)) {
                    respond402(response, path, "resource mismatch");
                    return;
                }
            } else {
                // 兼容旧 V1 头
                payloadV1 = PaymentPayload.fromHeader(legacyHeader);

                if (!Objects.equals(payloadV1.payload.get("resource"), path)) {
                    respond402(response, path, "resource mismatch");
                    return;
                }
                headerForFacilitator = legacyHeader;
            }

            vr = facilitator.verify(headerForFacilitator, buildRequirements(path));
        } catch (IllegalArgumentException ex) {
            // Malformed payment header - client error
            respond402(response, path, "malformed X-PAYMENT header");
            return;
        } catch (IOException ex) {
            // Network/communication error with facilitator - server error
            response.setStatus(HttpServletResponse.SC_INTERNAL_SERVER_ERROR);
            response.setContentType("application/json");
            try {
                response.getWriter().write("{\"error\":\"Payment verification failed: " + ex.getMessage() + "\"}");
            } catch (IOException writeEx) {
                // If we can't write the response, at least set the status
                System.err.println("Failed to write error response: " + writeEx.getMessage());
            }
            return;
        } catch (Exception ex) {
            // Other unexpected errors - server error
            response.setStatus(HttpServletResponse.SC_INTERNAL_SERVER_ERROR);
            response.setContentType("application/json");
            try {
                response.getWriter().write("{\"error\":\"Internal server error during payment verification\"}");
            } catch (IOException writeEx) {
                System.err.println("Failed to write error response: " + writeEx.getMessage());
            }
            return;
        }

        if (!vr.isValid) {
            respond402(response, path, vr.invalidReason);
            return;
        }

        /* -------- payment verified → continue business logic ----------- */
        chain.doFilter(req, res);

        /* -------- settlement (return errors to user) ------------- */
        // Don't settle if response failed (4xx/5xx status codes)
        if (response.getStatus() >= 400) {
            return;
        }

        try {
            SettlementResponse sr = facilitator.settle(headerForFacilitator, buildRequirements(path));
            if (sr == null || !sr.success) {
                // Settlement failed - return 402 if headers not sent yet
                if (!response.isCommitted()) {
                    String errorMsg = sr != null && sr.error != null ? sr.error : "settlement failed";
                    respond402(response, path, errorMsg);
                }
                return;
            }
            
            // Settlement succeeded - add settlement response header (base64-encoded JSON) 
            try {
                // Extract payer from payment payload (wallet address of person making payment)
                String payer = extractPayerFromPayload(
                    payloadV1 != null ? payloadV1 : (payloadV2 != null ? toV1Payload(payloadV2) : null)
                );

                // Legacy header (for existing clients)
                String base64LegacyHeader = createPaymentResponseHeader(sr, payer);
                response.setHeader("X-PAYMENT-RESPONSE", base64LegacyHeader);

                // New V2-style header aligned with Go SDK
                String base64V2Header = createPaymentResponseHeaderV2(sr, payer);
                response.setHeader("PAYMENT-RESPONSE", base64V2Header);

                // Set CORS header to expose settlement headers to browser clients
                response.setHeader(
                    "Access-Control-Expose-Headers",
                    "X-PAYMENT-RESPONSE, PAYMENT-RESPONSE"
                );
            } catch (Exception ex) {
                // If header creation fails, return 500
                if (!response.isCommitted()) {
                    response.setStatus(HttpServletResponse.SC_INTERNAL_SERVER_ERROR);
                    response.setContentType("application/json");
                    try {
                        response.getWriter().write("{\"error\":\"Failed to create settlement response header\"}");
                    } catch (IOException writeEx) {
                        System.err.println("Failed to write error response: " + writeEx.getMessage());
                    }
                }
                return;
            }
        } catch (Exception ex) {
            // Network/communication errors during settlement - return 402
            if (!response.isCommitted()) {
                respond402(response, path, "settlement error: " + ex.getMessage());
            }
            return;
        }
    }

    /* ------------------------------------------------ helpers ---------- */

    /** Build a V1 PaymentRequirements object for the given path and price. */
    private PaymentRequirements buildRequirements(String path) {
        PaymentRequirements pr = new PaymentRequirements();
        pr.scheme            = "exact";
        pr.network           = "base-sepolia";
        pr.maxAmountRequired = priceTable.get(path).toString();
        pr.asset             = "USDC";               // adjust for your token
        pr.resource          = path;
        pr.mimeType          = "application/json";
        pr.payTo             = payTo;
        pr.maxTimeoutSeconds = 30;
        return pr;
    }

    /** Build a V2 PaymentRequirements object for the given path and price. */
    private PaymentRequirementsV2 buildRequirementsV2(String path) {
        PaymentRequirementsV2 pr = new PaymentRequirementsV2();
        pr.scheme            = "exact";
        pr.network           = "base-sepolia";
        pr.amount            = priceTable.get(path).toString();
        pr.asset             = "USDC";
        pr.payTo             = payTo;
        pr.maxTimeoutSeconds = 30;

        // extra.resourceUrl 与 Go 侧保持一致语义
        Map<String, Object> extra = new java.util.HashMap<>();
        extra.put("resourceUrl", path);
        pr.extra = extra;

        return pr;
    }

    /** Create a V2 PaymentRequired object. */
    private PaymentRequiredV2 buildPaymentRequiredV2(String path, String error) {
        PaymentRequiredV2 pr = new PaymentRequiredV2();
        pr.x402Version = 2;
        pr.error       = error;

        ResourceInfo resourceInfo = new ResourceInfo();
        resourceInfo.url       = path;
        resourceInfo.mimeType  = "application/json";
        pr.resource            = resourceInfo;

        pr.accepts.add(buildRequirementsV2(path));
        pr.extensions = null;

        return pr;
    }

    /** Create a base64-encoded legacy payment response header. */
    private String createPaymentResponseHeader(SettlementResponse sr, String payer) throws Exception {
        SettlementResponseHeader settlementHeader = new SettlementResponseHeader(
            true,
            sr.txHash != null ? sr.txHash : "",
            sr.networkId != null ? sr.networkId : "",
            payer
        );
        
        String jsonString = Json.MAPPER.writeValueAsString(settlementHeader);
        return Base64.getEncoder().encodeToString(jsonString.getBytes(StandardCharsets.UTF_8));
    }

    /** Create a base64-encoded V2-style settlement response header. */
    private String createPaymentResponseHeaderV2(SettlementResponse sr, String payer) throws Exception {
        SettlementResponseV2 settlement = new SettlementResponseV2();
        settlement.success     = sr.success;
        settlement.transaction = sr.txHash;
        settlement.network     = sr.networkId;
        settlement.payer       = payer;
        settlement.errorReason = sr.error;

        String jsonString = Json.MAPPER.writeValueAsString(settlement);
        return Base64.getEncoder().encodeToString(jsonString.getBytes(StandardCharsets.UTF_8));
    }

    /** Extract the payer wallet address from payment payload. */
    private String extractPayerFromPayload(PaymentPayload payload) {
        try {
            // Convert the generic payload map to a typed ExactSchemePayload
            ExactSchemePayload exactPayload = Json.MAPPER.convertValue(payload.payload, ExactSchemePayload.class);
            return exactPayload.authorization != null ? exactPayload.authorization.from : null;
        } catch (Exception ex) {
            // If conversion fails, fall back to manual extraction for compatibility
            try {
                Object authorization = payload.payload.get("authorization");
                if (authorization instanceof Map) {
                    Object from = ((Map<?, ?>) authorization).get("from");
                    return from instanceof String ? (String) from : null;
                }
            } catch (Exception ignored) {
                // Ignore any extraction errors
            }
            return null;
        }
    }

    /** Write a JSON 402 response. */
    private void respond402(HttpServletResponse resp,
                            String             path,
                            String             error)
            throws IOException {

        resp.setStatus(HttpServletResponse.SC_PAYMENT_REQUIRED);
        resp.setContentType("application/json");

        // V2 header (primary for new clients, aligned with Go SDK)
        PaymentRequiredV2 v2 = buildPaymentRequiredV2(path, error);
        String v2Json;
        try {
            v2Json = Json.MAPPER.writeValueAsString(v2);
            String v2Base64 = Base64.getEncoder().encodeToString(
                    v2Json.getBytes(StandardCharsets.UTF_8)
            );
            resp.setHeader("PAYMENT-REQUIRED", v2Base64);
        } catch (IOException e) {
            // If V2 header creation fails, fall back to legacy body only
            v2Json = null;
        }

        // Legacy V1 JSON body kept for backwards compatibility with existing Java clients
        PaymentRequiredResponse v1 = new PaymentRequiredResponse();
        v1.x402Version = 1;
        v1.accepts.add(buildRequirements(path));
        v1.error = error;

        resp.getWriter().write(Json.MAPPER.writeValueAsString(v1));
    }

    /**
     * Convert a V2 payload into a minimal V1-style payload for payer extraction,
     * reusing the existing extraction logic.
     */
    private PaymentPayload toV1Payload(PaymentPayloadV2 v2) {
        if (v2 == null) {
            return null;
        }
        PaymentPayload p = new PaymentPayload();
        p.x402Version = v2.x402Version;
        p.scheme      = v2.accepted != null ? v2.accepted.scheme : null;
        p.network     = v2.accepted != null ? v2.accepted.network : null;
        p.payload     = v2.payload;
        return p;
    }
}
