package com.cubevlmu.lodestar_update_client;

import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;

import androidx.annotation.NonNull;
import androidx.core.content.FileProvider;

import java.io.File;

import io.flutter.embedding.engine.plugins.FlutterPlugin;
import io.flutter.plugin.common.MethodCall;
import io.flutter.plugin.common.MethodChannel;
import io.flutter.plugin.common.MethodChannel.MethodCallHandler;
import io.flutter.plugin.common.MethodChannel.Result;

public final class LodestarUpdateClientPlugin implements FlutterPlugin, MethodCallHandler {
    private MethodChannel channel;
    private Context applicationContext;

    @Override
    public void onAttachedToEngine(@NonNull FlutterPluginBinding binding) {
        applicationContext = binding.getApplicationContext();
        channel = new MethodChannel(binding.getBinaryMessenger(), "lodestar_update_client");
        channel.setMethodCallHandler(this);
    }

    @Override
    public void onDetachedFromEngine(@NonNull FlutterPluginBinding binding) {
        if (channel != null) {
            channel.setMethodCallHandler(null);
            channel = null;
        }
        applicationContext = null;
    }

    @Override
    public void onMethodCall(@NonNull MethodCall call, @NonNull Result result) {
        if (applicationContext == null) {
            result.error("NO_CONTEXT", "Android context is not available.", null);
            return;
        }

        switch (call.method) {
            case "installApk":
                installApk(call, result);
                break;
            case "canRequestPackageInstalls":
                result.success(canRequestPackageInstalls());
                break;
            case "openInstallPermissionSettings":
                openInstallPermissionSettings(result);
                break;
            default:
                result.notImplemented();
                break;
        }
    }

    private void installApk(@NonNull MethodCall call, @NonNull Result result) {
        String path = call.argument("path");
        if (path == null || path.trim().isEmpty()) {
            result.error("BAD_ARGUMENT", "APK path is empty.", null);
            return;
        }

        File apkFile = new File(path);
        if (!apkFile.exists() || !apkFile.isFile()) {
            result.error("FILE_NOT_FOUND", "APK file does not exist: " + path, null);
            return;
        }

        try {
            String authority = applicationContext.getPackageName() + ".lodestar_update_client.fileprovider";
            Uri uri = FileProvider.getUriForFile(applicationContext, authority, apkFile);
            Intent intent = new Intent(Intent.ACTION_VIEW);
            intent.setDataAndType(uri, "application/vnd.android.package-archive");
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
            applicationContext.startActivity(intent);
            result.success(null);
        } catch (Exception e) {
            result.error("INSTALL_APK_FAILED", e.getMessage(), null);
        }
    }

    private boolean canRequestPackageInstalls() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return true;
        }
        return applicationContext.getPackageManager().canRequestPackageInstalls();
    }

    private void openInstallPermissionSettings(@NonNull Result result) {
        try {
            Intent intent;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                Uri uri = Uri.parse("package:" + applicationContext.getPackageName());
                intent = new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, uri);
            } else {
                intent = new Intent(Settings.ACTION_SECURITY_SETTINGS);
            }
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            applicationContext.startActivity(intent);
            result.success(null);
        } catch (Exception e) {
            result.error("OPEN_SETTINGS_FAILED", e.getMessage(), null);
        }
    }
}
