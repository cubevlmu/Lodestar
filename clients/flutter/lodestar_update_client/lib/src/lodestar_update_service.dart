import 'lodestar_update_client_core.dart';
import 'lodestar_update_installer.dart';
import 'lodestar_update_models.dart';

class LodestarUpdateService {
  const LodestarUpdateService(this.client);

  final LodestarUpdateClient client;

  Future<CheckUpdateResponse> check({
    String? version,
    int? versionCode,
    String? clientId,
    String? installId,
    String? deviceId,
    String? osVersion,
  }) {
    return client.check(
      version: version,
      versionCode: versionCode,
      clientId: clientId,
      installId: installId,
      deviceId: deviceId,
      osVersion: osVersion,
    );
  }

  Future<DownloadedUpdatePackage> download({
    required UpdateInfo update,
    void Function(DownloadProgress progress)? onProgress,
    bool Function()? shouldCancel,
  }) {
    return client.downloadAndVerify(
      update,
      onProgress: onProgress,
      shouldCancel: shouldCancel,
    );
  }

  /// Android only. Downloads, verifies and opens system APK installer.
  Future<DownloadedUpdatePackage> downloadAndInstallAndroid({
    required UpdateInfo update,
    void Function(DownloadProgress progress)? onProgress,
    bool Function()? shouldCancel,
  }) async {
    final package = await download(
      update: update,
      onProgress: onProgress,
      shouldCancel: shouldCancel,
    );
    await LodestarUpdateInstaller.installApk(package.filePath);
    return package;
  }
}
