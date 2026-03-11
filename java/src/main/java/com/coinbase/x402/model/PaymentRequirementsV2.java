package com.coinbase.x402.model;

import java.util.Map;

/**
 * V2 payment requirements structure, aligned with Go types.PaymentRequirements.
 */
public class PaymentRequirementsV2 {
    public String scheme;
    public String network;
    public String asset;
    public String amount;
    public String payTo;
    public int    maxTimeoutSeconds;
    public Map<String, Object> extra;
}

