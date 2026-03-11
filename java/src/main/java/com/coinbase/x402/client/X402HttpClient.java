package com.coinbase.x402.client;

import com.coinbase.x402.crypto.CryptoSigner;
import com.coinbase.x402.crypto.CryptoSignException;
import com.coinbase.x402.model.PaymentPayload;
import com.coinbase.x402.model.PaymentPayloadV2;
import com.coinbase.x402.model.PaymentRequirementsV2;
import com.coinbase.x402.model.ResourceInfo;

import java.io.IOException;
import java.math.BigInteger;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.LinkedHashMap;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

/**
     * Convenience wrapper that builds an HTTP request with properly-formed
     * V2 payment headers for the “exact” EVM scheme on Base Sepolia.
 *
 * You provide a {@link CryptoSigner} implementation to actually sign the
 * payment payload (e.g. using web3j). Everything else is generic JSON + Base64.
 */
public class X402HttpClient {

    private final HttpClient http = HttpClient.newHttpClient();
    // Default to V2 protocol; V1 header is still emitted for backwards compatibility.
    private final int    x402Version = 2;
    private final String scheme      = "exact";
    private final String network     = "base-sepolia";

    private final CryptoSigner signer;

    /**
     * Creates a new X402 HTTP client with the specified crypto signer.
     *
     * @param signer the crypto signer for signing payment headers
     */
    public X402HttpClient(CryptoSigner signer) {
        this.signer = signer;
    }

    /**
     * Protected method that can be overridden in tests to mock HTTP responses.
     *
     * @param request the HTTP request to send
     * @return the HTTP response
     * @throws IOException if an I/O error occurs
     * @throws InterruptedException if the request is interrupted
     */
    protected HttpResponse<String> sendRequest(HttpRequest request) throws IOException, InterruptedException {
        return http.send(request, HttpResponse.BodyHandlers.ofString());
    }

    /**
     * Build and execute a <strong>GET</strong> request that includes payment
     * headers proving the caller intends to pay {@code amount} of {@code assetContract}
     * to {@code payTo}.
     *
     * 协议对齐 Go V2：
     * - 首选：PAYMENT-SIGNATURE: base64(json(PaymentPayloadV2))
     * - 兼容：X-PAYMENT: base64(json(PaymentPayload V1))
     */
    public HttpResponse<String> get(URI uri,
                                    BigInteger amount,
                                    String assetContract,
                                    String payTo)
            throws IOException, InterruptedException {

        /* ---------- Build scheme-specific payload map ------------------- */
        Map<String,Object> pl = new LinkedHashMap<>();
        pl.put("amount",   amount.toString());
        pl.put("asset",    assetContract);
        pl.put("payTo",    payTo);
        pl.put("resource", uri.getPath());
        pl.put("nonce",    UUID.randomUUID().toString());
        try {
            pl.put("signature", signer.sign(pl));      // <-- signer injected
        } catch (CryptoSignException e) {
            throw new RuntimeException("Failed to sign payment payload", e);
        }
        /* ---------------------------------------------------------------- */

        // -------------------- V2 payload ---------------------------------
        PaymentRequirementsV2 accepted = new PaymentRequirementsV2();
        accepted.scheme            = scheme;
        accepted.network           = network;
        accepted.asset             = assetContract;
        accepted.amount            = amount.toString();
        accepted.payTo             = payTo;
        accepted.maxTimeoutSeconds = 30;

        Map<String, Object> extra = new HashMap<>();
        extra.put("resourceUrl", uri.toString());
        accepted.extra = extra;

        ResourceInfo resourceInfo = new ResourceInfo();
        resourceInfo.url = uri.toString();
        resourceInfo.mimeType = "application/json";

        PaymentPayloadV2 v2 = new PaymentPayloadV2();
        v2.x402Version = x402Version;
        v2.payload     = pl;
        v2.accepted    = accepted;
        v2.resource    = resourceInfo;
        v2.extensions  = null;

        // -------------------- V1 payload (compat) ------------------------
        PaymentPayload v1 = new PaymentPayload();
        v1.x402Version = 1;
        v1.scheme      = scheme;
        v1.network     = network;
        v1.payload     = pl;

        HttpRequest req = HttpRequest.newBuilder()
                .uri(uri)
                .header("PAYMENT-SIGNATURE", v2.toHeader())
                .header("X-PAYMENT", v1.toHeader())
                .GET()
                .build();

        return sendRequest(req);
    }
}
