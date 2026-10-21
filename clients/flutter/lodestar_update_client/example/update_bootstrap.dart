import 'package:lodestar_update_client/lodestar_update_client.dart';

class UpdateBootstrapResult {
  const UpdateBootstrapResult({
    required this.canEnterApp,
    required this.message,
    this.response,
  });

  final bool canEnterApp;
  final String message;
  final CheckUpdateResponse? response;
}

Future<UpdateBootstrapResult> checkAppUpdate() async {
  final client = LodestarUpdateClient(
    config: const LodestarUpdateConfig(
      baseUrl: 'https://update.example.com',
      appId: 'com.example.class_tool',
      clientSecret: '替换成服务端项目 client_secret',
      ed25519PublicKey: '替换成服务端项目 ed25519_public_key',
      platform: 'android',
      arch: 'universal',
      channel: 'stable',
    ),
  );

  try {
    final installId = await LodestarIdentity.getOrCreateInstallId();
    final resp = await client.check(
      clientId: installId,
      installId: installId,
    );

    if (!resp.legal.allowed) {
      return UpdateBootstrapResult(
        canEnterApp: false,
        message: resp.legal.message,
        response: resp,
      );
    }

    if (!resp.update.available) {
      return UpdateBootstrapResult(
        canEnterApp: true,
        message: 'already_latest',
        response: resp,
      );
    }

    return UpdateBootstrapResult(
      canEnterApp: true,
      message: resp.update.isForce ? 'force_update' : 'recommended_update',
      response: resp,
    );
  } finally {
    client.close();
  }
}

Future<void> downloadAndInstall(CheckUpdateResponse response) async {
  final client = LodestarUpdateClient(
    config: const LodestarUpdateConfig(
      baseUrl: 'https://update.example.com',
      appId: 'com.example.class_tool',
      clientSecret: '替换成服务端项目 client_secret',
      ed25519PublicKey: '替换成服务端项目 ed25519_public_key',
      platform: 'android',
      arch: 'universal',
      channel: 'stable',
    ),
  );

  try {
    final package = await client.downloadAndVerify(
      response.update,
      onProgress: (progress) {
        // 接到你自己的下载进度 UI。
        // progress.fraction 为 null 时表示服务器没有返回总大小。
      },
    );

    final allowed = await LodestarUpdateInstaller.canRequestPackageInstalls();
    if (!allowed) {
      await LodestarUpdateInstaller.openInstallPermissionSettings();
      return;
    }

    await LodestarUpdateInstaller.installApk(package.filePath);
  } finally {
    client.close();
  }
}
