import 'dart:io' show Platform;

import 'package:device_info_plus/device_info_plus.dart';
import 'package:package_info_plus/package_info_plus.dart';

class GxLocalAppInfo {
  const GxLocalAppInfo({
    required this.version,
    required this.versionCode,
    required this.packageName,
  });

  final String version;
  final int versionCode;
  final String packageName;
}

Future<GxLocalAppInfo> readLocalAppInfo() async {
  final info = await PackageInfo.fromPlatform();
  return GxLocalAppInfo(
    version: info.version,
    versionCode: int.tryParse(info.buildNumber) ?? 0,
    packageName: info.packageName,
  );
}

Future<String> readOsVersion() async {
  final plugin = DeviceInfoPlugin();
  if (Platform.isAndroid) {
    final info = await plugin.androidInfo;
    return 'Android ${info.version.release} SDK ${info.version.sdkInt}';
  }
  if (Platform.isIOS) {
    final info = await plugin.iosInfo;
    return '${info.systemName} ${info.systemVersion}';
  }
  if (Platform.isWindows) {
    final info = await plugin.windowsInfo;
    return 'Windows ${info.majorVersion}.${info.minorVersion}.${info.buildNumber}';
  }
  if (Platform.isMacOS) {
    final info = await plugin.macOsInfo;
    return 'macOS ${info.majorVersion}.${info.minorVersion}.${info.patchVersion}';
  }
  if (Platform.isLinux) {
    final info = await plugin.linuxInfo;
    return '${info.name} ${info.version ?? ''}'.trim();
  }
  return Platform.operatingSystemVersion;
}
