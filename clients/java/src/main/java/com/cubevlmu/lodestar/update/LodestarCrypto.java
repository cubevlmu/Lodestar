package com.cubevlmu.lodestar.update;

import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.Signature;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

/**
 * Signing helpers shared by every Lodestar client.
 *
 * <p>The request signature is {@code base64(HMAC-SHA256(secret, payload))} where
 * the payload is:
 *
 * <pre>
 * METHOD
 * /path
 * timestamp
 * nonce
 * sha256(body) hex
 * </pre>
 *
 * <p>The key is the raw bytes of {@code client_secret}: the server does not try
 * to base64-decode it, so neither should the client.
 */
public final class LodestarCrypto {

    private static final byte[] ED25519_SPKI_PREFIX = {
        0x30, 0x2a, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x03, 0x21, 0x00
    };

    private LodestarCrypto() {
    }

    public static byte[] hmacKey(String secret) {
        return secret.getBytes(StandardCharsets.UTF_8);
    }

    public static String bodySha256Hex(byte[] body) {
        try {
            java.security.MessageDigest digest = java.security.MessageDigest.getInstance("SHA-256");
            return toHex(digest.digest(body));
        } catch (Exception e) {
            throw new LodestarUpdateException("SHA-256 is unavailable", e);
        }
    }

    public static String signingPayload(String method, String path, String timestamp, String nonce, byte[] body) {
        return String.join(
                "\n",
                method.toUpperCase(),
                path,
                timestamp,
                nonce,
                bodySha256Hex(body));
    }

    public static String signRequest(
            String secret, String method, String path, String timestamp, String nonce, byte[] body) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(hmacKey(secret), "HmacSHA256"));
            byte[] digest = mac.doFinal(signingPayload(method, path, timestamp, nonce, body).getBytes(StandardCharsets.UTF_8));
            return Base64.getEncoder().encodeToString(digest);
        } catch (Exception e) {
            throw new LodestarUpdateException("HMAC-SHA256 is unavailable", e);
        }
    }

    /** Builds the exact byte sequence the server signs with its Ed25519 key. */
    public static String canonicalManifest(String appId, UpdateModels.UpdateInfo update) {
        return "lodestar-manifest-v1\n"
                + "app_id=" + appId + "\n"
                + "version=" + update.version() + "\n"
                + "version_code=" + update.versionCode() + "\n"
                + "platform=" + update.platform() + "\n"
                + "arch=" + update.arch() + "\n"
                + "channel=" + update.channel() + "\n"
                + "package_url=" + update.packageInfo().downloadUrl() + "\n"
                + "package_size=" + update.packageInfo().size() + "\n"
                + "package_sha256=" + update.packageInfo().sha256() + "\n"
                + "force=" + update.force() + "\n"
                + "min_supported_code=" + update.minSupportedCode() + "\n";
    }

    /**
     * Verifies the manifest signature. Returns true when the update is not
     * available (nothing to verify) or when the signature matches.
     */
    public static boolean verifyManifest(String publicKeyBase64, String appId, UpdateModels.UpdateInfo update) {
        if (!update.available()) {
            return true;
        }
        if (update.manifestSignature() == null || update.manifestSignature().isEmpty()) {
            return false;
        }
        byte[] publicKey = Base64.getDecoder().decode(publicKeyBase64.trim());
        byte[] signature = Base64.getDecoder().decode(update.manifestSignature());
        if (publicKey.length != 32) {
            throw new LodestarUpdateException("Invalid Ed25519 public key length: " + publicKey.length);
        }
        if (signature.length != 64) {
            throw new LodestarUpdateException("Invalid Ed25519 signature length: " + signature.length);
        }
        try {
            byte[] spki = new byte[ED25519_SPKI_PREFIX.length + publicKey.length];
            System.arraycopy(ED25519_SPKI_PREFIX, 0, spki, 0, ED25519_SPKI_PREFIX.length);
            System.arraycopy(publicKey, 0, spki, ED25519_SPKI_PREFIX.length, publicKey.length);
            PublicKey key = KeyFactory.getInstance("Ed25519").generatePublic(new X509EncodedKeySpec(spki));
            Signature verifier = Signature.getInstance("Ed25519");
            verifier.initVerify(key);
            verifier.update(canonicalManifest(appId, update).getBytes(StandardCharsets.UTF_8));
            return verifier.verify(signature);
        } catch (Exception e) {
            throw new LodestarUpdateException("Ed25519 verification failed", e);
        }
    }

    public static String toHex(byte[] bytes) {
        StringBuilder builder = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) {
            builder.append(Character.forDigit((b >> 4) & 0xf, 16));
            builder.append(Character.forDigit(b & 0xf, 16));
        }
        return builder.toString();
    }
}
