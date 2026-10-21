import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart' as crypto;
import 'package:cryptography/cryptography.dart';

import 'lodestar_update_exception.dart';
import 'lodestar_update_models.dart';

/// The server signs with the raw bytes of `client_secret`; it does not try to
/// base64-decode it. Doing so here would silently derive a different key for
/// secrets that happen to look like base64.
List<int> hmacKey(String secret) => utf8.encode(secret);

String bodySha256Hex(List<int> body) {
  return crypto.sha256.convert(body).toString();
}

String requestSigningPayload({
  required String method,
  required String path,
  required String timestamp,
  required String nonce,
  required List<int> body,
}) {
  return <String>[
    method.toUpperCase(),
    path,
    timestamp,
    nonce,
    bodySha256Hex(body),
  ].join('\n');
}

String signRequestHmac({
  required String secret,
  required String method,
  required String path,
  required String timestamp,
  required String nonce,
  required List<int> body,
}) {
  final mac = crypto.Hmac(crypto.sha256, hmacKey(secret));
  final payload = requestSigningPayload(
    method: method,
    path: path,
    timestamp: timestamp,
    nonce: nonce,
    body: body,
  );
  return base64.encode(mac.convert(utf8.encode(payload)).bytes);
}

String canonicalManifestPayload({
  required String appId,
  required UpdateInfo update,
}) {
  return 'lodestar-manifest-v1\n'
      'app_id=$appId\n'
      'version=${update.version}\n'
      'version_code=${update.versionCode}\n'
      'platform=${update.platform}\n'
      'arch=${update.arch}\n'
      'channel=${update.channel}\n'
      'package_url=${update.package.downloadUrl}\n'
      'package_size=${update.package.size}\n'
      'package_sha256=${update.package.sha256}\n'
      'force=${update.force}\n'
      'min_supported_code=${update.minSupportedCode}\n';
}

Future<bool> verifyManifestSignature({
  required String publicKeyBase64,
  required String appId,
  required UpdateInfo update,
}) async {
  if (!update.available) return true;
  if (update.manifestSignature.isEmpty) return false;

  final publicKeyBytes = _decodeBase64Strict(
    publicKeyBase64,
    fieldName: 'ed25519PublicKey',
  );
  final signatureBytes = _decodeBase64Strict(
    update.manifestSignature,
    fieldName: 'manifestSignature',
  );

  if (publicKeyBytes.length != 32) {
    throw const LodestarUpdateException('Invalid Ed25519 public key length.');
  }
  if (signatureBytes.length != 64) {
    throw const LodestarUpdateException('Invalid Ed25519 signature length.');
  }

  final algorithm = Ed25519();
  final payload = utf8.encode(
    canonicalManifestPayload(appId: appId, update: update),
  );

  return algorithm.verify(
    payload,
    signature: Signature(
      Uint8List.fromList(signatureBytes),
      publicKey: SimplePublicKey(
        Uint8List.fromList(publicKeyBytes),
        type: KeyPairType.ed25519,
      ),
    ),
  );
}

List<int> _decodeBase64Strict(String value, {required String fieldName}) {
  try {
    return base64.decode(value);
  } catch (e) {
    throw LodestarUpdateException('Invalid base64 value: $fieldName.', cause: e);
  }
}
