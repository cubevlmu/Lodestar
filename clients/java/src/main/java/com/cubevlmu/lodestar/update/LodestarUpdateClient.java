package com.cubevlmu.lodestar.update;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardCopyOption;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;
import java.util.function.BooleanSupplier;

/**
 * Signed update checks and verified package downloads for Lodestar.
 *
 * <pre>{@code
 * var config = LodestarUpdateConfig.builder()
 *     .baseUrl("https://lodestar.example.com")
 *     .appId("demo.app")
 *     .clientSecret(secret)
 *     .ed25519PublicKey(publicKey)
 *     .arch("amd64")
 *     .build();
 *
 * try (var client = new LodestarUpdateClient(config)) {
 *     var response = client.check("1.4.1", 1_004_001, null);
 *     if (response.hasUpdate() && response.update().isForce()) {
 *         var pkg = client.downloadAndVerify(response.update(), Path.of("updates"), null, null);
 *     }
 * }
 * }</pre>
 */
public final class LodestarUpdateClient implements AutoCloseable {

    /** Called while downloading; {@code total} is -1 when the server omits a length. */
    public interface ProgressListener {
        void onProgress(long received, long total);
    }

    private final LodestarUpdateConfig config;
    private final HttpClient http;

    public LodestarUpdateClient(LodestarUpdateConfig config) {
        this.config = config;
        this.http = HttpClient.newBuilder()
                .connectTimeout(config.connectTimeout())
                .followRedirects(HttpClient.Redirect.NORMAL)
                .build();
    }

    public LodestarUpdateConfig config() {
        return config;
    }

    /** Calls {@code POST /api/v1/check} with HMAC headers. */
    public UpdateModels.CheckResponse check(String version, long versionCode, String clientId) {
        return check(version, versionCode, clientId, null, null, null);
    }

    public UpdateModels.CheckResponse check(
            String version,
            long versionCode,
            String clientId,
            String installId,
            String deviceId,
            String osVersion) {

        URI uri = config.resolve("/api/v1/check");
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("app_id", config.appId());
        body.put("platform", config.platform());
        body.put("arch", config.arch());
        body.put("channel", config.channel());
        body.put("version", version);
        body.put("version_code", versionCode);
        putIfPresent(body, "client_id", clientId);
        putIfPresent(body, "install_id", installId);
        putIfPresent(body, "device_id", deviceId);
        putIfPresent(body, "os_version", osVersion);

        byte[] payload = Json.write(body).getBytes(java.nio.charset.StandardCharsets.UTF_8);
        String timestamp = String.valueOf(Instant.now().getEpochSecond());
        String nonce = UUID.randomUUID().toString();
        String signature = LodestarCrypto.signRequest(
                config.clientSecret(), "POST", uri.getPath(), timestamp, nonce, payload);

        HttpRequest request = HttpRequest.newBuilder(uri)
                .timeout(config.requestTimeout())
                .header("Content-Type", "application/json")
                .header("Accept", "application/json")
                .header("X-App-Id", config.appId())
                .header("X-Timestamp", timestamp)
                .header("X-Nonce", nonce)
                .header("X-Signature", signature)
                .POST(HttpRequest.BodyPublishers.ofByteArray(payload))
                .build();

        HttpResponse<String> response = send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() < 200 || response.statusCode() >= 300) {
            throw new LodestarHttpException(
                    "Check update failed with HTTP " + response.statusCode(), response.statusCode(), response.body());
        }
        return UpdateModels.CheckResponse.from(Json.parseObject(response.body()));
    }

    /** Throws when the manifest signature does not verify. */
    public void verifyManifest(UpdateModels.UpdateInfo update) {
        if (!LodestarCrypto.verifyManifest(config.ed25519PublicKey(), config.appId(), update)) {
            throw new LodestarUpdateException("Update manifest signature verification failed");
        }
    }

    public UpdateModels.DownloadedPackage downloadAndVerify(UpdateModels.UpdateInfo update, Path targetDir) {
        return downloadAndVerify(update, targetDir, null, null);
    }

    /**
     * Downloads the package, checks its SHA-256 and returns the local file. The
     * manifest is verified first; a failed hash check deletes the file.
     */
    public UpdateModels.DownloadedPackage downloadAndVerify(
            UpdateModels.UpdateInfo update,
            Path targetDir,
            ProgressListener listener,
            BooleanSupplier shouldCancel) {

        if (!update.available()) {
            throw new LodestarUpdateException("No update package is available");
        }
        if (update.packageInfo().downloadUrl().isEmpty()) {
            throw new LodestarUpdateException("Update package has an empty download URL");
        }
        if (update.packageInfo().sha256().isEmpty()) {
            throw new LodestarUpdateException("Update package has an empty SHA-256");
        }
        verifyManifest(update);
        cancelIfRequested(shouldCancel);

        URI uri = URI.create(update.packageInfo().downloadUrl());
        HttpRequest request = HttpRequest.newBuilder(uri)
                .timeout(config.requestTimeout())
                .header("Accept", "*/*")
                .GET()
                .build();

        try {
            Files.createDirectories(targetDir);
            String fileName = safeFileName(update.packageInfo().fileName(), "update_" + update.versionCode());
            Path target = targetDir.resolve(fileName);
            Path temp = targetDir.resolve(fileName + ".part");

            HttpResponse<InputStream> response = send(request, HttpResponse.BodyHandlers.ofInputStream());
            if (response.statusCode() < 200 || response.statusCode() >= 300) {
                throw new LodestarHttpException(
                        "Download failed with HTTP " + response.statusCode(), response.statusCode(), "");
            }

            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            long received = 0;
            long total = response.headers().firstValueAsLong("content-length").orElse(update.packageInfo().size());

            try (InputStream in = response.body(); OutputStream out = Files.newOutputStream(temp)) {
                byte[] buffer = new byte[64 * 1024];
                int read;
                while ((read = in.read(buffer)) != -1) {
                    cancelIfRequested(shouldCancel);
                    out.write(buffer, 0, read);
                    digest.update(buffer, 0, read);
                    received += read;
                    if (listener != null) {
                        listener.onProgress(received, total);
                    }
                }
            }

            String actual = LodestarCrypto.toHex(digest.digest());
            if (!actual.equalsIgnoreCase(update.packageInfo().sha256())) {
                Files.deleteIfExists(temp);
                throw new LodestarUpdateException(
                        "SHA-256 mismatch: expected " + update.packageInfo().sha256() + ", got " + actual);
            }
            Files.move(temp, target, StandardCopyOption.REPLACE_EXISTING);
            return new UpdateModels.DownloadedPackage(target, fileName, received, actual, update);
        } catch (IOException | java.security.NoSuchAlgorithmException e) {
            throw new LodestarUpdateException("Download failed: " + e.getMessage(), e);
        }
    }

    private <T> HttpResponse<T> send(HttpRequest request, HttpResponse.BodyHandler<T> handler) {
        try {
            return http.send(request, handler);
        } catch (IOException e) {
            throw new LodestarUpdateException("Request failed: " + e.getMessage(), e);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new LodestarUpdateException("Request interrupted", e);
        }
    }

    private static void cancelIfRequested(BooleanSupplier shouldCancel) {
        if (shouldCancel != null && shouldCancel.getAsBoolean()) {
            throw new LodestarUpdateException("Update cancelled");
        }
    }

    private static void putIfPresent(Map<String, Object> body, String key, String value) {
        if (value != null && !value.isBlank()) {
            body.put(key, value);
        }
    }

    private static String safeFileName(String name, String fallback) {
        String cleaned = name == null ? "" : name.replaceAll("[^A-Za-z0-9._\\-+]", "_");
        if (cleaned.isBlank() || ".".equals(cleaned) || "..".equals(cleaned)) {
            return fallback;
        }
        return cleaned;
    }

    @Override
    public void close() {
        // HttpClient has no close(); kept for try-with-resources ergonomics.
    }
}
