class CheckUpdateRequest {
  const CheckUpdateRequest({
    required this.appId,
    required this.platform,
    required this.arch,
    required this.channel,
    required this.version,
    required this.versionCode,
    this.clientId,
    this.installId,
    this.deviceId,
    this.osVersion,
  });

  final String appId;
  final String platform;
  final String arch;
  final String channel;
  final String version;
  final int versionCode;
  final String? clientId;
  final String? installId;
  final String? deviceId;
  final String? osVersion;

  Map<String, dynamic> toJson() {
    return <String, dynamic>{
      'app_id': appId,
      'platform': platform,
      'arch': arch,
      'channel': channel,
      'version': version,
      'version_code': versionCode,
      if (clientId != null && clientId!.isNotEmpty) 'client_id': clientId,
      if (installId != null && installId!.isNotEmpty) 'install_id': installId,
      if (deviceId != null && deviceId!.isNotEmpty) 'device_id': deviceId,
      if (osVersion != null && osVersion!.isNotEmpty) 'os_version': osVersion,
    };
  }
}

class CheckUpdateResponse {
  const CheckUpdateResponse({
    required this.requestId,
    required this.serverTime,
    required this.legal,
    required this.update,
  });

  final String requestId;
  final int serverTime;
  final LegalInfo legal;
  final UpdateInfo update;

  bool get isAllowed => legal.allowed;
  bool get hasUpdate => update.available;
  bool get mustUpdate => !legal.allowed || update.isForce;

  factory CheckUpdateResponse.fromJson(Map<String, dynamic> json) {
    return CheckUpdateResponse(
      requestId: json['request_id'] as String? ?? '',
      serverTime: _asInt(json['server_time']),
      legal: LegalInfo.fromJson(
        json['legal'] as Map<String, dynamic>? ?? const <String, dynamic>{},
      ),
      update: UpdateInfo.fromJson(
        json['update'] as Map<String, dynamic>? ??
            const <String, dynamic>{
              'available': false,
              'update_type': 'none',
            },
      ),
    );
  }
}

class LegalInfo {
  const LegalInfo({
    required this.allowed,
    required this.code,
    required this.message,
  });

  final bool allowed;
  final String code;
  final String message;

  factory LegalInfo.fromJson(Map<String, dynamic> json) {
    return LegalInfo(
      allowed: json['allowed'] as bool? ?? false,
      code: json['code'] as String? ?? '',
      message: json['message'] as String? ?? '',
    );
  }
}

class UpdateInfo {
  const UpdateInfo({
    required this.available,
    required this.updateType,
    required this.version,
    required this.versionCode,
    required this.platform,
    required this.arch,
    required this.channel,
    required this.title,
    required this.changelog,
    required this.minSupportedCode,
    required this.force,
    required this.package,
    required this.manifestSignature,
    required this.createdAt,
  });

  final bool available;
  final String updateType;
  final String version;
  final int versionCode;
  final String platform;
  final String arch;
  final String channel;
  final String title;
  final String changelog;
  final int minSupportedCode;
  final bool force;
  final PackageInfo package;
  final String manifestSignature;
  final DateTime? createdAt;

  bool get isForce => updateType == 'force' || force;
  bool get isRecommended => updateType == 'recommended' && !isForce;

  factory UpdateInfo.fromJson(Map<String, dynamic> json) {
    return UpdateInfo(
      available: json['available'] as bool? ?? false,
      updateType: json['update_type'] as String? ?? 'none',
      version: json['version'] as String? ?? '',
      versionCode: _asInt(json['version_code']),
      platform: json['platform'] as String? ?? '',
      arch: json['arch'] as String? ?? '',
      channel: json['channel'] as String? ?? '',
      title: json['title'] as String? ?? '',
      changelog: json['changelog'] as String? ?? '',
      minSupportedCode: _asInt(json['min_supported_code']),
      force: json['force'] as bool? ?? false,
      package: PackageInfo.fromJson(
        json['package'] as Map<String, dynamic>? ?? const <String, dynamic>{},
      ),
      manifestSignature: json['manifest_signature'] as String? ?? '',
      createdAt: _parseDateTime(json['created_at']),
    );
  }
}

class PackageInfo {
  const PackageInfo({
    required this.id,
    required this.fileName,
    required this.downloadUrl,
    required this.size,
    required this.sha256,
  });

  final int id;
  final String fileName;
  final String downloadUrl;
  final int size;
  final String sha256;

  factory PackageInfo.fromJson(Map<String, dynamic> json) {
    return PackageInfo(
      id: _asInt(json['id']),
      fileName: json['file_name'] as String? ?? '',
      downloadUrl: json['download_url'] as String? ?? '',
      size: _asInt(json['size']),
      sha256: (json['sha256'] as String? ?? '').toLowerCase(),
    );
  }
}

class DownloadedUpdatePackage {
  const DownloadedUpdatePackage({
    required this.filePath,
    required this.fileName,
    required this.size,
    required this.sha256,
    required this.update,
  });

  final String filePath;
  final String fileName;
  final int size;
  final String sha256;
  final UpdateInfo update;
}

class DownloadProgress {
  const DownloadProgress({
    required this.received,
    required this.total,
  });

  final int received;
  final int total;

  double? get fraction {
    if (total <= 0) return null;
    return received / total;
  }
}

int _asInt(Object? value) {
  if (value == null) return 0;
  if (value is int) return value;
  if (value is num) return value.toInt();
  if (value is String) return int.tryParse(value) ?? 0;
  return 0;
}

DateTime? _parseDateTime(Object? value) {
  if (value is! String || value.isEmpty) return null;
  return DateTime.tryParse(value);
}
