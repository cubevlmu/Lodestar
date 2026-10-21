import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:convert/convert.dart' as convert;
import 'package:crypto/crypto.dart' as crypto;
import 'package:http/http.dart' as http;
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:uuid/uuid.dart';

import 'lodestar_update_config.dart';
import 'lodestar_update_crypto.dart';
import 'lodestar_update_device.dart';
import 'lodestar_update_exception.dart';
import 'lodestar_update_models.dart';

class LodestarUpdateClient {
  LodestarUpdateClient({
    required this.config,
    http.Client? httpClient,
  }) : _httpClient = httpClient ?? http.Client();

  final LodestarUpdateConfig config;
  final http.Client _httpClient;
  final Uuid _uuid = const Uuid();

  /// Calls POST /api/v1/check with HMAC headers.
  ///
  /// [clientId] should be a stable anonymous id stored locally by the app.
  /// It is used by the server for stable grayscale rollout.
  Future<CheckUpdateResponse> check({
    String? version,
    int? versionCode,
    String? clientId,
    String? installId,
    String? deviceId,
    String? osVersion,
  }) async {
    final localInfo = version == null || versionCode == null
        ? await readLocalAppInfo()
        : null;
    final resolvedOsVersion = osVersion ?? await readOsVersion();

    final request = CheckUpdateRequest(
      appId: config.appId,
      platform: config.effectivePlatform,
      arch: config.arch,
      channel: config.channel,
      version: version ?? localInfo!.version,
      versionCode: versionCode ?? localInfo!.versionCode,
      clientId: clientId,
      installId: installId,
      deviceId: deviceId,
      osVersion: resolvedOsVersion,
    );

    final uri = config.resolveApi('/api/v1/check');
    final body = utf8.encode(jsonEncode(request.toJson()));
    final timestamp =
        (DateTime.now().millisecondsSinceEpoch ~/ 1000).toString();
    final nonce = _uuid.v4();
    final signature = signRequestHmac(
      secret: config.clientSecret,
      method: 'POST',
      path: uri.path,
      timestamp: timestamp,
      nonce: nonce,
      body: body,
    );

    final response = await _httpClient
        .post(
          uri,
          headers: <String, String>{
            'Content-Type': 'application/json',
            'Accept': 'application/json',
            'X-App-Id': config.appId,
            'X-Timestamp': timestamp,
            'X-Nonce': nonce,
            'X-Signature': signature,
          },
          body: body,
        )
        .timeout(config.connectTimeout + config.receiveTimeout);

    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw LodestarHttpException(
        'Check update failed.',
        statusCode: response.statusCode,
        body: response.body,
      );
    }

    final decoded = jsonDecode(utf8.decode(response.bodyBytes));
    if (decoded is! Map<String, dynamic>) {
      throw const LodestarUpdateException('Bad check update response.');
    }
    return CheckUpdateResponse.fromJson(decoded);
  }

  /// Verifies server signed update manifest.
  ///
  /// Call this before showing force-update download actions if you want to fail
  /// early when a proxy or attacker tampers with response fields.
  Future<void> verifyManifest(UpdateInfo update) async {
    final ok = await verifyManifestSignature(
      publicKeyBase64: config.ed25519PublicKey,
      appId: config.appId,
      update: update,
    );
    if (!ok) {
      throw const LodestarUpdateException(
          'Update manifest signature verification failed.');
    }
  }

  /// Downloads update package and verifies SHA256.
  ///
  /// The update manifest is verified before download. The file hash is verified
  /// after download. A failed verification deletes the downloaded file.
  Future<DownloadedUpdatePackage> downloadAndVerify(
    UpdateInfo update, {
    void Function(DownloadProgress progress)? onProgress,
    bool Function()? shouldCancel,
    String? saveAsFileName,
  }) async {
    void checkCancelled() {
      if (shouldCancel?.call() ?? false) {
        throw const LodestarUpdateCancelledException();
      }
    }

    if (!update.available) {
      throw const LodestarUpdateException('No update package is available.');
    }
    if (update.package.downloadUrl.isEmpty) {
      throw const LodestarUpdateException('Update package has empty download URL.');
    }
    if (update.package.sha256.isEmpty) {
      throw const LodestarUpdateException('Update package has empty SHA256.');
    }

    await verifyManifest(update);
    checkCancelled();

    final uri = Uri.parse(update.package.downloadUrl);
    final request = http.Request('GET', uri);
    final response = await _httpClient
        .send(request)
        .timeout(config.connectTimeout + config.receiveTimeout);
    checkCancelled();

    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw LodestarHttpException(
        'Download update package failed.',
        statusCode: response.statusCode,
      );
    }

    final dir = await _downloadDir();
    await dir.create(recursive: true);
    final fileName = _safeFileName(
      saveAsFileName ?? update.package.fileName,
      fallback: 'update_${update.versionCode}.apk',
    );
    final file = File(p.join(dir.path, fileName));
    if (await file.exists()) {
      await file.delete();
    }

    final digestSink = convert.AccumulatorSink<crypto.Digest>();
    final digestInput = crypto.sha256.startChunkedConversion(digestSink);
    final sink = file.openWrite();
    var received = 0;
    final total = response.contentLength ?? update.package.size;

    try {
      await for (final chunk in response.stream) {
        checkCancelled();
        received += chunk.length;
        digestInput.add(chunk);
        sink.add(chunk);
        onProgress?.call(DownloadProgress(received: received, total: total));
        checkCancelled();
      }
      await sink.flush();
      await sink.close();
      digestInput.close();
    } on LodestarUpdateCancelledException {
      await sink.close();
      if (await file.exists()) await file.delete();
      rethrow;
    } catch (e) {
      await sink.close();
      if (await file.exists()) await file.delete();
      throw LodestarUpdateException('Download update package interrupted.', cause: e);
    }

    final sha256 = digestSink.events.single.toString().toLowerCase();
    if (sha256 != update.package.sha256.toLowerCase()) {
      if (await file.exists()) await file.delete();
      throw LodestarUpdateException(
        'Update package SHA256 verification failed. expected=${update.package.sha256}, actual=$sha256',
      );
    }

    return DownloadedUpdatePackage(
      filePath: file.path,
      fileName: fileName,
      size: received,
      sha256: sha256,
      update: update,
    );
  }

  Future<Directory> _downloadDir() async {
    final base = await getTemporaryDirectory();
    return Directory(p.join(base.path, config.downloadDirName));
  }

  String _safeFileName(String value, {required String fallback}) {
    final name = p.basename(value).trim();
    if (name.isEmpty || name == '.' || name == '..') return fallback;
    return name.replaceAll(RegExp(r'[^a-zA-Z0-9._\-+]'), '_');
  }

  void close() {
    _httpClient.close();
  }
}
