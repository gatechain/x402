package com.coinbase.x402.client;

import com.coinbase.x402.model.PaymentPayload;
import com.coinbase.x402.model.PaymentRequirements;
import com.coinbase.x402.util.Json;
import com.fasterxml.jackson.core.type.TypeReference;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.*;

/** Synchronous facilitator client using Java 17 HttpClient, adapted for Gate Web3 OpenAPI. */
public class HttpFacilitatorClient implements FacilitatorClient {

    /** Default facilitator URL (Gate Web3 OpenAPI Testnet). */
    public static final String DEFAULT_FACILITATOR_URL = "https://openapi-test.gateweb3.cc/api/v1/x402";

    // Gate Web3 signing path and logical target URIs (used for x-target-uri)
    private static final String GATE_WEB3_SIGNING_PATH         = "/api/v1/x402";
    private static final String TARGET_URI_SUPPORTED           = "/v1/x402/supported";
    private static final String TARGET_URI_VERIFY              = "/v1/x402/verify";
    private static final String TARGET_URI_SETTLE              = "/v1/x402/settle";
    private static final String ENV_GATE_WEB3_API_KEY          = "GATE_WEB3_API_KEY";
    private static final String ENV_GATE_WEB3_API_SECRET       = "GATE_WEB3_API_SECRET";
    private static final String ENV_GATE_WEB3_PASSPHRASE       = "GATE_WEB3_PASSPHRASE";
    private static final String ENV_GATE_WEB3_REAL_IP          = "GATE_WEB3_REAL_IP";
    private static final String DEFAULT_GATE_WEB3_FORWARDED_FOR = "127.0.0.1";
    private static final String DEFAULT_GATE_WEB3_PASSPHRASE   = "";

    private final HttpClient http =
            HttpClient.newBuilder()
                      .connectTimeout(Duration.ofSeconds(30))
                      .build();

    /**
     * Base URL of the facilitator service (single OpenAPI endpoint, no trailing slash trimming).
     * For Gate Web3 this is {@link #DEFAULT_FACILITATOR_URL}.
     */
    private final String baseUrl;

    /**
     * Creates a new HTTP facilitator client.
     *
     * @param baseUrl the base URL of the facilitator service.
     *                If null or blank, {@link #DEFAULT_FACILITATOR_URL} is used.
     */
    public HttpFacilitatorClient(String baseUrl) {
        String url = baseUrl;
        if (url == null || url.isBlank()) {
            url = DEFAULT_FACILITATOR_URL;
        }
        this.baseUrl = url;
    }

    /* ------------------------------------------------ gate signing -------- */

    private static final class GateWeb3Credentials {
        final String apiKey;
        final String apiSecret;
        final String passphrase;
        final String realIp;

        private GateWeb3Credentials(String apiKey, String apiSecret, String passphrase, String realIp) {
            this.apiKey = apiKey;
            this.apiSecret = apiSecret;
            this.passphrase = passphrase;
            this.realIp = realIp;
        }
    }

    private static Optional<GateWeb3Credentials> loadGateWeb3Credentials() {
        String ak = Optional.ofNullable(System.getenv(ENV_GATE_WEB3_API_KEY))
                .map(String::trim)
                .orElse("");
        String sk = Optional.ofNullable(System.getenv(ENV_GATE_WEB3_API_SECRET))
                .map(String::trim)
                .orElse("");
        if (ak.isEmpty() || sk.isEmpty()) {
            return Optional.empty();
        }

        String pass = Optional.ofNullable(System.getenv(ENV_GATE_WEB3_PASSPHRASE))
                .orElse(DEFAULT_GATE_WEB3_PASSPHRASE);
        String realIp = Optional.ofNullable(System.getenv(ENV_GATE_WEB3_REAL_IP))
                .orElse(DEFAULT_GATE_WEB3_FORWARDED_FOR);

        return Optional.of(new GateWeb3Credentials(ak, sk, pass, realIp));
    }

    /**
     * Apply Gate Web3 HMAC-SHA256 signature headers to a request builder.
     *
     * PREHASH = &lt;timestamp&gt;&lt;GATE_WEB3_SIGNING_PATH&gt;&lt;rawBody&gt;
     * Signature = Base64(HMAC_SHA256(SK, PREHASH))
     */
    private static void applyGateWeb3Signature(HttpRequest.Builder builder,
                                               byte[] body,
                                               String targetUri) {
        Optional<GateWeb3Credentials> credsOpt = loadGateWeb3Credentials();
        if (credsOpt.isEmpty()) {
            // No credentials configured; leave signing to external infrastructure if any.
            return;
        }
        GateWeb3Credentials creds = credsOpt.get();

        long timestamp = System.currentTimeMillis();
        String prehash = timestamp + GATE_WEB3_SIGNING_PATH + new String(body, StandardCharsets.UTF_8);

        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(creds.apiSecret.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
            byte[] sigBytes = mac.doFinal(prehash.getBytes(StandardCharsets.UTF_8));
            String signature = Base64.getEncoder().encodeToString(sigBytes);

            builder.header("X-Api-Key", creds.apiKey);
            builder.header("X-Timestamp", Long.toString(timestamp));
            builder.header("X-Signature", signature);
            if (!creds.passphrase.isEmpty()) {
                builder.header("X-Passphrase", creds.passphrase);
            }
            if (!creds.realIp.isEmpty()) {
                builder.header("X-Forwarded-For", creds.realIp);
            }
            builder.header("X-Request-Id", UUID.randomUUID().toString());
            // x-target-uri expects no leading slash
            String normalizedTarget = targetUri.startsWith("/")
                    ? targetUri.substring(1)
                    : targetUri;
            builder.header("x-target-uri", normalizedTarget);
        } catch (Exception e) {
            throw new IllegalStateException("Failed to apply Gate Web3 signature", e);
        }
    }

    /* ------------------------------------------------ API envelope -------- */

    private static final class FacilitatorApiResponse<T> {
        public int code;
        public String msg;
        public T data;
    }

    private static final class SupportedKindDto {
        public int x402Version;
        public String scheme;
        public String network;
        public Map<String, Object> extra;
    }

    private static final class SupportedResponseDto {
        public List<SupportedKindDto> kinds;
        public List<String> extensions;
        public Map<String, List<String>> signers;
    }

    /* ------------------------------------------------ verify ------------- */

    @Override
    public VerificationResponse verify(String paymentHeader,
                                       PaymentRequirements req)
            throws IOException, InterruptedException {

        // Decode header into structured payment payload (V1-style)
        PaymentPayload payload = PaymentPayload.fromHeader(paymentHeader);

        Map<String, Object> params = new LinkedHashMap<>();
        params.put("x402Version", payload.x402Version);
        params.put("paymentPayload", payload.payload);
        params.put("paymentRequirements", req);

        Map<String, Object> body = Map.of(
                "action", "x402.verify",
                "params", params
        );

        String json = Json.MAPPER.writeValueAsString(body);

        HttpRequest.Builder builder = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl))
                .header("Content-Type", "application/json")
                .timeout(Duration.ofSeconds(30))
                .POST(HttpRequest.BodyPublishers.ofString(json));

        applyGateWeb3Signature(builder, json.getBytes(StandardCharsets.UTF_8), TARGET_URI_VERIFY);

        HttpRequest request = builder.build();

        HttpResponse<String> response = http.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() != 200) {
            throw new IOException("HTTP " + response.statusCode() + ": " + response.body());
        }

        FacilitatorApiResponse<VerificationResponse> apiResp =
                Json.MAPPER.readValue(response.body(),
                        new TypeReference<FacilitatorApiResponse<VerificationResponse>>() {});

        if (apiResp.code != 0) {
            throw new IOException("Facilitator verify failed (code=" + apiResp.code + ", msg=" + apiResp.msg + ")");
        }

        return apiResp.data;
    }

    /* ------------------------------------------------ settle ------------- */

    @Override
    public SettlementResponse settle(String paymentHeader,
                                     PaymentRequirements req)
            throws IOException, InterruptedException {

        // Decode header into structured payment payload (V1-style)
        PaymentPayload payload = PaymentPayload.fromHeader(paymentHeader);

        Map<String, Object> params = new LinkedHashMap<>();
        params.put("x402Version", payload.x402Version);
        params.put("paymentPayload", payload.payload);
        params.put("paymentRequirements", req);

        Map<String, Object> body = Map.of(
                "action", "x402.settle",
                "params", params
        );

        String json = Json.MAPPER.writeValueAsString(body);

        HttpRequest.Builder builder = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl))
                .header("Content-Type", "application/json")
                .timeout(Duration.ofSeconds(30))
                .POST(HttpRequest.BodyPublishers.ofString(json));

        applyGateWeb3Signature(builder, json.getBytes(StandardCharsets.UTF_8), TARGET_URI_SETTLE);

        HttpRequest request = builder.build();

        HttpResponse<String> response = http.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() != 200) {
            throw new IOException("HTTP " + response.statusCode() + ": " + response.body());
        }

        FacilitatorApiResponse<SettlementResponse> apiResp =
                Json.MAPPER.readValue(response.body(),
                        new TypeReference<FacilitatorApiResponse<SettlementResponse>>() {});

        if (apiResp.code != 0) {
            String reason = apiResp.data != null && apiResp.data.error != null
                    ? apiResp.data.error
                    : apiResp.msg;
            throw new IOException("Facilitator settle failed (code=" + apiResp.code + ", msg=" + reason + ")");
        }

        return apiResp.data;
    }

    /* ------------------------------------------------ supported ---------- */

    @Override
    public Set<Kind> supported() throws IOException, InterruptedException {
        Map<String, Object> body = Map.of(
                "action", "x402.supported",
                "params", Map.of()
        );

        String json = Json.MAPPER.writeValueAsString(body);

        HttpRequest.Builder builder = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl))
                .header("Content-Type", "application/json")
                .timeout(Duration.ofSeconds(30))
                .POST(HttpRequest.BodyPublishers.ofString(json));

        applyGateWeb3Signature(builder, json.getBytes(StandardCharsets.UTF_8), TARGET_URI_SUPPORTED);

        HttpRequest request = builder.build();

        HttpResponse<String> response = http.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() != 200) {
            throw new IOException("HTTP " + response.statusCode() + ": " + response.body());
        }

        FacilitatorApiResponse<SupportedResponseDto> apiResp =
                Json.MAPPER.readValue(response.body(),
                        new TypeReference<FacilitatorApiResponse<SupportedResponseDto>>() {});

        if (apiResp.code != 0) {
            throw new IOException("Facilitator supported failed (code=" + apiResp.code + ", msg=" + apiResp.msg + ")");
        }

        SupportedResponseDto data = apiResp.data;
        if (data == null || data.kinds == null) {
            return Set.of();
        }

        Set<Kind> out = new HashSet<>();
        for (SupportedKindDto kind : data.kinds) {
            if (kind != null && kind.scheme != null && kind.network != null) {
                out.add(new Kind(kind.scheme, kind.network));
            }
        }
        return out;
    }
}
