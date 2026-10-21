package com.cubevlmu.lodestar.update;

import java.time.Instant;
import java.util.Map;

/** Response models returned by {@code POST /api/v1/check}. */
public final class UpdateModels {

    private UpdateModels() {
    }

    public record Legal(boolean allowed, String code, String message) {
        static Legal from(Map<String, Object> json) {
            return new Legal(
                    bool(json.get("allowed")),
                    string(json.get("code")),
                    string(json.get("message")));
        }
    }

    public record PackageInfo(long id, String fileName, String downloadUrl, long size, String sha256) {
        static PackageInfo from(Map<String, Object> json) {
            return new PackageInfo(
                    number(json.get("id")),
                    string(json.get("file_name")),
                    string(json.get("download_url")),
                    number(json.get("size")),
                    string(json.get("sha256")).toLowerCase());
        }
    }

    public record UpdateInfo(
            boolean available,
            String updateType,
            String version,
            long versionCode,
            String platform,
            String arch,
            String channel,
            String title,
            String changelog,
            long minSupportedCode,
            boolean force,
            PackageInfo packageInfo,
            String manifestSignature,
            Instant createdAt) {

        /** Force when the server marked it, or when the policy calls it "force". */
        public boolean isForce() {
            return force || "force".equals(updateType);
        }

        public boolean isRecommended() {
            return "recommended".equals(updateType) && !isForce();
        }

        static UpdateInfo from(Map<String, Object> json) {
            Map<String, Object> pkg = object(json.get("package"));
            return new UpdateInfo(
                    bool(json.get("available")),
                    string(json.get("update_type")),
                    string(json.get("version")),
                    number(json.get("version_code")),
                    string(json.get("platform")),
                    string(json.get("arch")),
                    string(json.get("channel")),
                    string(json.get("title")),
                    string(json.get("changelog")),
                    number(json.get("min_supported_code")),
                    bool(json.get("force")),
                    PackageInfo.from(pkg),
                    string(json.get("manifest_signature")),
                    instant(json.get("created_at")));
        }
    }

    public record CheckResponse(String requestId, long serverTime, Legal legal, UpdateInfo update) {

        public boolean isAllowed() {
            return legal.allowed();
        }

        public boolean hasUpdate() {
            return update.available();
        }

        /** A blocked client or a forced update must be applied before use. */
        public boolean mustUpdate() {
            return !legal.allowed() || update.isForce();
        }

        static CheckResponse from(Map<String, Object> json) {
            Map<String, Object> update = object(json.get("update"));
            if (update.isEmpty()) {
                update = Map.of("available", false, "update_type", "none");
            }
            return new CheckResponse(
                    string(json.get("request_id")),
                    number(json.get("server_time")),
                    Legal.from(object(json.get("legal"))),
                    UpdateInfo.from(update));
        }
    }

    public record DownloadedPackage(
            java.nio.file.Path file, String fileName, long size, String sha256, UpdateInfo update) {
    }

    // ---------- tolerant field readers ----------

    @SuppressWarnings("unchecked")
    static Map<String, Object> object(Object value) {
        return value instanceof Map ? (Map<String, Object>) value : Map.of();
    }

    static String string(Object value) {
        return value == null ? "" : String.valueOf(value);
    }

    static long number(Object value) {
        if (value instanceof Number n) {
            return n.longValue();
        }
        if (value instanceof String s && !s.isEmpty()) {
            try {
                return Long.parseLong(s);
            } catch (NumberFormatException ignored) {
                return 0;
            }
        }
        return 0;
    }

    static boolean bool(Object value) {
        if (value instanceof Boolean b) {
            return b;
        }
        return value instanceof String s && Boolean.parseBoolean(s);
    }

    static Instant instant(Object value) {
        if (value instanceof String s && !s.isEmpty()) {
            try {
                return Instant.parse(s);
            } catch (RuntimeException ignored) {
                return null;
            }
        }
        return null;
    }
}
