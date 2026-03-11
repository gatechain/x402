package com.coinbase.x402.model;

/**
 * V2 settlement response structure, aligned with Go x402.SettleResponse.
 */
public class SettlementResponseV2 {
    public boolean success;
    public String  errorReason;
    public String  payer;
    public String  network;
    public String  transaction;
}

