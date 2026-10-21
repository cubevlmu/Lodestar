import 'dart:io' show Platform;

class LodestarUpdateConfig {
  const LodestarUpdateConfig({
    required this.baseUrl,
    required this.appId,
    required this.clientSecret,
    required this.ed25519PublicKey,
    this.platform,
    this.arch = 'universal',
    this.channel = 'stable',
    this.connectTimeout = const Duration(seconds: 10),
    this.receiveTimeout = const Duration(seconds: 30),
    this.downloadDirName = 'gx_update',
  });

  /// Server root, for example: https://update.example.com
  final String baseUrl;

  /// Must match server project app_id.
  final String appId;

  /// Returned by server when creating the project. Used for HMAC request signing.
  /// This is not a hard security boundary because clients can be reverse engineered.
  final String clientSecret;

  /// Returned by server when creating the project. Used to verify update manifests.
  final String ed25519PublicKey;

  /// android / ios / windows / macos / linux.
  final String? platform;

  /// universal / arm64-v8a / armeabi-v7a / x64 etc. Must match server release arch.
  final String arch;

  /// stable / beta / dev etc. Must match server release channel.
  final String channel;

  final Duration connectTimeout;
  final Duration receiveTimeout;

  /// Subdirectory under app temporary directory for downloaded update packages.
  final String downloadDirName;

  String get effectivePlatform => platform ?? detectPlatform();

  Uri resolveApi(String path) {
    final trimmed = baseUrl.endsWith('/')
        ? baseUrl.substring(0, baseUrl.length - 1)
        : baseUrl;
    final normalized = path.startsWith('/') ? path : '/$path';
    return Uri.parse('$trimmed$normalized');
  }

  static String detectPlatform() {
    if (Platform.isAndroid) return 'android';
    if (Platform.isIOS) return 'ios';
    if (Platform.isWindows) return 'windows';
    if (Platform.isMacOS) return 'macos';
    if (Platform.isLinux) return 'linux';
    return 'unknown';
  }
}
