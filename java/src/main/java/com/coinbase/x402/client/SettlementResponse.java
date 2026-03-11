package com.coinbase.x402.client;

/** JSON returned by POST /settle on the facilitator. */
public class SettlementResponse {
    /** Whether the payment settlement succeeded. */
    public boolean success;
    
    /** Error message if settlement failed. */
    public String  error;
    
    /** Transaction hash of the settled payment. */
    public String  txHash;
    
    /** Network ID where the settlement occurred. */
    public String  networkId;

    /**
     * Optional bag of facilitator-specific metadata.
     * This is a direct mapping of the Go {@code x402.SettleResponse.Extra} field.
     */
    public java.util.Map<String, Object> extra;
}
