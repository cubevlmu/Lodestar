package com.cubevlmu.lodestar.update;

import java.net.URI;
import java.time.Duration;

/** Immutable client configuration. */
public final class LodestarUpdateConfig {

    private final String baseUrl;
    private final String appId;
    private final String clientSecret;
    private final String ed25519PublicKey;
    private final String platform;
    private final String arch;
    private final String channel;
    private final Duration connectTimeout;
    private final Duration requestTimeout;

    private LodestarUpdateConfig(Builder builder) {
        if (builder.baseUrl == null || builder.baseUrl.isBlank()) {
            throw new IllegalArgumentException("baseUrl is required");
        }
        if (builder.appId == null || builder.appId.isBlank()) {
            throw new IllegalArgumentException("appId is required");
        }
        if (builder.clientSecret == null || builder.clientSecret.isBlank()) {
            throw new IllegalArgumentException("clientSecret is required");
        }
        if (builder.ed25519PublicKey == null || builder.ed25519PublicKey.isBlank()) {
            throw new IllegalArgumentException("ed25519PublicKey is required");
        }
        this.baseUrl = stripTrailingSlash(builder.baseUrl.trim());
        this.appId = builder.appId.trim();
        this.clientSecret = builder.clientSecret;
        this.ed25519PublicKey = builder.ed25519PublicKey.trim();
        this.platform = builder.platform != null ? builder.platform : detectPlatform();
        this.arch = builder.arch != null ? builder.arch : "universal";
        this.channel = builder.channel != null ? builder.channel : "stable";
        this.connectTimeout = builder.connectTimeout;
        this.requestTimeout = builder.requestTimeout;
    }

    public static Builder builder() {
        return new Builder();
    }

    public String baseUrl() {
        return baseUrl;
    }

    public String appId() {
        return appId;
    }

    public String clientSecret() {
        return clientSecret;
    }

    public String ed25519PublicKey() {
        return ed25519PublicKey;
    }

    public String platform() {
        return platform;
    }

    public String arch() {
        return arch;
    }

    public String channel() {
        return channel;
    }

    public Duration connectTimeout() {
        return connectTimeout;
    }

    public Duration requestTimeout() {
        return requestTimeout;
    }

    /** Resolves a server path such as {@code /api/v1/check} against the base URL. */
    public URI resolve(String path) {
        String normalized = path.startsWith("/") ? path : "/" + path;
        return URI.create(baseUrl + normalized);
    }

    private static String stripTrailingSlash(String value) {
        String result = value;
        while (result.endsWith("/")) {
            result = result.substring(0, result.length() - 1);
        }
        return result;
    }

    /** Maps the JVM's OS name to the platform token Lodestar expects. */
    public static String detectPlatform() {
        String os = System.getProperty("os.name", "").toLowerCase();
        if (os.contains("win")) {
            return "windows";
        }
        if (os.contains("mac") || os.contains("darwin")) {
            return "darwin";
        }
        if (os.contains("linux")) {
            return "linux";
        }
        return "unknown";
    }

    public static final class Builder {
        private String baseUrl;
        private String appId;
        private String clientSecret;
        private String ed25519PublicKey;
        private String platform;
        private String arch;
        private String channel;
        private Duration connectTimeout = Duration.ofSeconds(10);
        private Duration requestTimeout = Duration.ofSeconds(30);

        public Builder baseUrl(String value) {
            this.baseUrl = value;
            return this;
        }

        public Builder appId(String value) {
            this.appId = value;
            return this;
        }

        public Builder clientSecret(String value) {
            this.clientSecret = value;
            return this;
        }

        public Builder ed25519PublicKey(String value) {
            this.ed25519PublicKey = value;
            return this;
        }

        public Builder platform(String value) {
            this.platform = value;
            return this;
        }

        public Builder arch(String value) {
            this.arch = value;
            return this;
        }

        public Builder channel(String value) {
            this.channel = value;
            return this;
        }

        public Builder connectTimeout(Duration value) {
            this.connectTimeout = value;
            return this;
        }

        public Builder requestTimeout(Duration value) {
            this.requestTimeout = value;
            return this;
        }

        public LodestarUpdateConfig build() {
            return new LodestarUpdateConfig(this);
        }
    }
}
