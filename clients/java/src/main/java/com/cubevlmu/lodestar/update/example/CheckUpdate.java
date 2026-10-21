package com.cubevlmu.lodestar.update.example;

import com.cubevlmu.lodestar.update.LodestarUpdateClient;
import com.cubevlmu.lodestar.update.LodestarUpdateConfig;
import com.cubevlmu.lodestar.update.UpdateModels;
import java.nio.file.Path;

/** Command line smoke test: check, verify the manifest, optionally download. */
public final class CheckUpdate {

    public static void main(String[] args) {
        String url = arg(args, "--url", "http://127.0.0.1:8080");
        String appId = arg(args, "--app", null);
        String secret = arg(args, "--secret", null);
        String publicKey = arg(args, "--pub", null);
        String version = arg(args, "--version", "1.0.0");
        long code = Long.parseLong(arg(args, "--code", "1000000"));
        String platform = arg(args, "--platform", null);
        String arch = arg(args, "--arch", "universal");
        String channel = arg(args, "--channel", "stable");
        String downloadDir = arg(args, "--download", null);

        if (appId == null || secret == null || publicKey == null) {
            System.err.println("usage: CheckUpdate --app <app_id> --secret <client_secret> --pub <ed25519_public_key>"
                    + " [--url ...] [--version ...] [--code ...] [--platform ...] [--arch ...] [--channel ...]"
                    + " [--download <dir>]");
            System.exit(2);
        }

        LodestarUpdateConfig config = LodestarUpdateConfig.builder()
                .baseUrl(url)
                .appId(appId)
                .clientSecret(secret)
                .ed25519PublicKey(publicKey)
                .platform(platform)
                .arch(arch)
                .channel(channel)
                .build();

        try (LodestarUpdateClient client = new LodestarUpdateClient(config)) {
            UpdateModels.CheckResponse response = client.check(version, code, "java-example");
            System.out.println("legal.allowed = " + response.legal().allowed());
            System.out.println("update.available = " + response.hasUpdate());
            if (!response.hasUpdate()) {
                System.out.println("already up to date");
                return;
            }
            UpdateModels.UpdateInfo update = response.update();
            System.out.println("version = " + update.version() + " (" + update.versionCode() + ")");
            System.out.println("force = " + update.isForce() + ", type = " + update.updateType());
            System.out.println("package = " + update.packageInfo().fileName()
                    + " (" + update.packageInfo().size() + " bytes)");

            client.verifyManifest(update);
            System.out.println("manifest signature = ok");

            if (downloadDir != null) {
                UpdateModels.DownloadedPackage pkg = client.downloadAndVerify(
                        update,
                        Path.of(downloadDir),
                        (received, total) -> System.out.println("  " + received + "/" + total),
                        null);
                System.out.println("downloaded = " + pkg.file() + " (" + pkg.sha256() + ")");
            }
        }
    }

    private static String arg(String[] args, String name, String fallback) {
        for (int i = 0; i < args.length - 1; i++) {
            if (args[i].equals(name)) {
                return args[i + 1];
            }
        }
        return fallback;
    }

    private CheckUpdate() {
    }
}
