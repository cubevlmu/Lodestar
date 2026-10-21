import 'dart:io' show Platform;

import 'package:flutter/services.dart';

import 'lodestar_update_exception.dart';

class LodestarUpdateInstaller {
  static const MethodChannel _channel = MethodChannel('lodestar_update_client');

  static bool get isAndroid => Platform.isAndroid;

  /// Opens Android package installer for an APK file.
  ///
  /// This only starts the system install flow. The user still needs to confirm.
  static Future<void> installApk(String apkPath) async {
    if (!Platform.isAndroid) {
      throw const LodestarUpdateException('installApk is only supported on Android.');
    }
    if (apkPath.trim().isEmpty) {
      throw const LodestarUpdateException('APK path is empty.');
    }
    await _channel.invokeMethod<void>('installApk', <String, Object?>{
      'path': apkPath,
    });
  }

  /// Returns whether current app can request package installs.
  ///
  /// On Android 8.0+, users may need to grant "install unknown apps" permission.
  static Future<bool> canRequestPackageInstalls() async {
    if (!Platform.isAndroid) return false;
    final result = await _channel.invokeMethod<bool>('canRequestPackageInstalls');
    return result ?? false;
  }

  /// Opens Android settings page for granting "install unknown apps" permission.
  static Future<void> openInstallPermissionSettings() async {
    if (!Platform.isAndroid) return;
    await _channel.invokeMethod<void>('openInstallPermissionSettings');
  }
}
