package com.cubevlmu.lodestar.update;

import java.nio.charset.StandardCharsets;
import java.util.Base64;

/**
 * Dependency-free test runner.
 *
 * <p>Run with {@code java -cp out com.cubevlmu.lodestar.update.SelfTest}; exits
 * non-zero on the first failure. Keeping this free of JUnit means the client
 * stays dependency-free and the test can run anywhere a JDK is installed.
 */
public final class SelfTest {

    private static int passed;

    public static void main(String[] args) {
        signingPayloadMatchesServer();
        hmacUsesRawSecret();
        base64LookingSecretIsNotDecoded();
        manifestCanonicalString();
        manifestRejectsWrongSignature();

        System.out.println(passed + " checks passed");
    }

    private static void signingPayloadMatchesServer() {
        byte[] body = "{\"app_id\":\"demo.app\",\"arch\":\"amd64\",\"channel\":\"stable\",\"platform\":\"linux\",\"version\":\"1.0.0\",\"version_code\":1000000}"
                .getBytes(StandardCharsets.UTF_8);
        String payload = LodestarCrypto.signingPayload("post", "/api/v1/check", "1700000000", "nonce-1", body);
        expect(
                payload.equals("POST\n/api/v1/check\n1700000000\nnonce-1\n"
                        + "8a88dcf3efd954856da26800fd6c395508f4c6b5d891ff1586ee8578253009b9"),
                "signing payload: " + payload);
    }

    private static void hmacUsesRawSecret() {
        byte[] body = "{\"app_id\":\"demo.app\",\"arch\":\"amd64\",\"channel\":\"stable\",\"platform\":\"linux\",\"version\":\"1.0.0\",\"version_code\":1000000}"
                .getBytes(StandardCharsets.UTF_8);
        String signature = LodestarCrypto.signRequest(
                "s3cr3t-key", "POST", "/api/v1/check", "1700000000", "nonce-1", body);
        expect(signature.equals("ZwVMKnpQgsB2YjLgGYpogIGq4DeR63ygFLWAKQywgzQ="), "signature: " + signature);
    }

    private static void base64LookingSecretIsNotDecoded() {
        // "c2VjcmV0" is valid base64 ("secret"); the server hashes the literal
        // characters, so the client must not decode them.
        expect(
                java.util.Arrays.equals(LodestarCrypto.hmacKey("c2VjcmV0"), "c2VjcmV0".getBytes(StandardCharsets.UTF_8)),
                "base64-looking secret must not be decoded");
    }

    private static void manifestCanonicalString() {
        UpdateModels.PackageInfo pkg =
                new UpdateModels.PackageInfo(1, "demo.apk", "https://example.com/demo.apk?token=abc", 12345, "abc123");
        UpdateModels.UpdateInfo update = new UpdateModels.UpdateInfo(
                true, "recommended", "1.4.1", 1004001, "android", "arm64", "stable",
                "Fixes", "- fix", 1000000, false, pkg, "", null);
        String canonical = LodestarCrypto.canonicalManifest("demo.app", update);
        String expected = "lodestar-manifest-v1\n"
                + "app_id=demo.app\n"
                + "version=1.4.1\n"
                + "version_code=1004001\n"
                + "platform=android\n"
                + "arch=arm64\n"
                + "channel=stable\n"
                + "package_url=https://example.com/demo.apk?token=abc\n"
                + "package_size=12345\n"
                + "package_sha256=abc123\n"
                + "force=false\n"
                + "min_supported_code=1000000\n";
        expect(canonical.equals(expected), "canonical manifest:/n" + canonical);
    }

    private static void manifestRejectsWrongSignature() {
        UpdateModels.PackageInfo pkg = new UpdateModels.PackageInfo(1, "a.bin", "https://example.com/a.bin", 1, "00");
        UpdateModels.UpdateInfo update = new UpdateModels.UpdateInfo(
                true, "force", "2.0.0", 2000000, "linux", "amd64", "stable",
                "", "", 0, true, pkg, Base64.getEncoder().encodeToString(new byte[64]), null);
        // A random 32-byte key and a zeroed signature must not verify.
        String fakeKey = Base64.getEncoder().encodeToString(new byte[32]);
        boolean verified;
        try {
            verified = LodestarCrypto.verifyManifest(fakeKey, "demo.app", update);
        } catch (RuntimeException e) {
            verified = false;
        }
        expect(!verified, "wrong signature must not verify");
    }

    private static void expect(boolean condition, String message) {
        if (!condition) {
            throw new AssertionError(message);
        }
        passed++;
    }

    private SelfTest() {
    }
}
