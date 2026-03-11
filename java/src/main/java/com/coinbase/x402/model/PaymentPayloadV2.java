package com.coinbase.x402.model;

import com.coinbase.x402.util.Json;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Map;

/**
 * V2 payment payload structure, aligned with Go types.PaymentPayload.
 */
public class PaymentPayloadV2 {
    public int x402Version;
    public Map<String, Object> payload;
    public PaymentRequirementsV2 accepted;
    public ResourceInfo resource;
    public Map<String, Object> extensions;

    /**
     * Serialise and base64-encode for the PAYMENT-SIGNATURE header.
     */
    public String toHeader() {
        try {
            String json = Json.MAPPER.writeValueAsString(this);
            return Base64.getEncoder().encodeToString(json.getBytes(StandardCharsets.UTF_8));
        } catch (IOException e) {
            throw new IllegalStateException("Unable to encode V2 payment header", e);
        }
    }

    /**
     * Decode from PAYMENT-SIGNATURE header.
     */
    public static PaymentPayloadV2 fromHeader(String header) throws IOException {
        byte[] decoded = Base64.getDecoder().decode(header);
        return Json.MAPPER.readValue(decoded, PaymentPayloadV2.class);
    }
}

