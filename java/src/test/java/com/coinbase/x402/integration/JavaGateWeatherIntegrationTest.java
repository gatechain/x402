package com.coinbase.x402.integration;

import com.coinbase.x402.client.*;
import com.coinbase.x402.crypto.CryptoSignException;
import com.coinbase.x402.crypto.CryptoSigner;
import com.coinbase.x402.model.PaymentRequirements;
import com.coinbase.x402.server.PaymentFilter;
import jakarta.servlet.http.HttpServlet;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.eclipse.jetty.server.Server;
import org.eclipse.jetty.servlet.FilterHolder;
import org.eclipse.jetty.servlet.ServletContextHandler;
import org.eclipse.jetty.servlet.ServletHolder;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.io.PrintWriter;
import java.math.BigInteger;
import java.net.URI;
import java.net.http.HttpResponse;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * Java 端到端集成测试：
 *
 * - 在本地起一个嵌入式 Jetty + {@link PaymentFilter} 的 server，filter 接入我们自己的 FacilitatorClient 实现
 * - 使用 {@link X402HttpClient}（集成了 CryptoSigner）作为客户端访问受保护的接口
 *
 * 这个测试主要验证：
 * - Java 客户端可以构造正确的 V2/V1 支付头（PAYMENT-SIGNATURE / X-PAYMENT）
 * - {@link PaymentFilter} 能解析支付头、调用 facilitator，并在成功时放行请求
 *
 * 注意：这里的 FacilitatorClient 是本地 stub，而不是真实 Gate OpenAPI，
 * 真正对接 Gate 在单元测试里会受到网络、IP 白名单、链上环境等限制。
 */
class JavaGateWeatherIntegrationTest {

    private static Server jetty;
    private static int    port;

    @BeforeAll
    static void startJetty() throws Exception {
        // -------- stub facilitator（本地内存实现）-----------------------
        FacilitatorClient stubFac = new FacilitatorClient() {
            @Override
            public VerificationResponse verify(String paymentHeader, PaymentRequirements req)
                    throws IOException, InterruptedException {
                VerificationResponse vr = new VerificationResponse();
                vr.isValid = true;         // 总是验证通过
                vr.invalidReason = null;
                return vr;
            }

            @Override
            public SettlementResponse settle(String paymentHeader, PaymentRequirements req)
                    throws IOException, InterruptedException {
                SettlementResponse sr = new SettlementResponse();
                sr.success = true;         // 总是结算成功
                sr.txHash = "0xmocktx";
                sr.networkId = "gatelayer_testnet";
                sr.error = null;
                return sr;
            }

            @Override
            public Set<Kind> supported() throws IOException, InterruptedException {
                // 声明支持 exact + base-sepolia（和默认 PaymentFilter 配置保持一致）
                return Set.of(new Kind("exact", "base-sepolia"));
            }
        };

        Map<String, BigInteger> priceTable = Map.of(
                "/private", BigInteger.valueOf(1_000) // 任意非零价格即可
        );

        // -------- Jetty server + PaymentFilter ----------------------------
        jetty = new Server(0); // 端口自动分配
        ServletContextHandler ctx = new ServletContextHandler();
        ctx.setContextPath("/");

        // 业务 servlet：只有通过 PaymentFilter 才能访问
        ctx.addServlet(new ServletHolder(new HttpServlet() {
            @Override
            protected void doGet(HttpServletRequest req, HttpServletResponse resp) throws IOException {
                resp.setContentType("application/json");
                try (PrintWriter w = resp.getWriter()) {
                    w.write("{\"ok\":true}");
                }
            }
        }), "/private");

        // 注册 PaymentFilter
        ctx.addFilter(
                new FilterHolder(new PaymentFilter("0xReceiver", priceTable, stubFac)),
                "/*",
                null
        );

        jetty.setHandler(ctx);
        jetty.start();
        port = jetty.getURI().getPort();
    }

    @AfterAll
    static void stopJetty() throws Exception {
        if (jetty != null) {
            jetty.stop();
        }
    }

    @Test
    void endToEnd_withJavaClientAndPaymentFilter() throws Exception {
        // -------- Java 客户端：使用 X402HttpClient + CryptoSigner ----------
        CryptoSigner signer = payload -> {
            // 这里不做真实链上签名，只返回一个固定签名字符串即可
            // 真正对接链上时，你可以在这里调用 web3j 等库完成 EIP-3009 / 712 签名
            return "0xMockSignature";
        };

        X402HttpClient client = new X402HttpClient(signer);

        URI uri = URI.create("http://localhost:" + port + "/private");
        BigInteger amount = BigInteger.valueOf(1_000);
        String assetContract = "USDC";
        String payTo = "0xReceiver";

        HttpResponse<String> response = client.get(uri, amount, assetContract, payTo);

        assertNotNull(response);
        assertEquals(200, response.statusCode());
        assertEquals("{\"ok\":true}", response.body());
    }
}

