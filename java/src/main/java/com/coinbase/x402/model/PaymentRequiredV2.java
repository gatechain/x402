package com.coinbase.x402.model;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * V2 HTTP 402 response structure, aligned with Go types.PaymentRequired.
 */
public class PaymentRequiredV2 {
    public int x402Version;
    public String error;
    public ResourceInfo resource;
    public List<PaymentRequirementsV2> accepts = new ArrayList<>();
    public Map<String, Object> extensions;
}

