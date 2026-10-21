import 'dart:convert';

import 'package:test/test.dart';
import 'package:lodestar_update_client/src/lodestar_update_crypto.dart';
import 'package:lodestar_update_client/src/lodestar_update_models.dart';

void main() {
  const body =
      '{"app_id":"demo.app","arch":"amd64","channel":"stable","platform":"linux","version":"1.0.0","version_code":1000000}';

  test('signing payload is method, path, timestamp, nonce, body hash', () {
    final payload = requestSigningPayload(
      method: 'post',
      path: '/api/v1/check',
      timestamp: '1700000000',
      nonce: 'nonce-1',
      body: utf8.encode(body),
    );
    expect(
      payload,
      'POST\n/api/v1/check\n1700000000\nnonce-1\n'
      '8a88dcf3efd954856da26800fd6c395508f4c6b5d891ff1586ee8578253009b9',
    );
  });

  test('HMAC uses the raw secret bytes', () {
    final signature = signRequestHmac(
      secret: 's3cr3t-key',
      method: 'POST',
      path: '/api/v1/check',
      timestamp: '1700000000',
      nonce: 'nonce-1',
      body: utf8.encode(body),
    );
    expect(signature, 'ZwVMKnpQgsB2YjLgGYpogIGq4DeR63ygFLWAKQywgzQ=');
  });

  test('a base64-looking secret is not decoded', () {
    // "c2VjcmV0" is valid base64 ("secret"); the server hashes the literal
    // characters, so the client must not decode them.
    final signature = signRequestHmac(
      secret: 'c2VjcmV0',
      method: 'POST',
      path: '/api/v1/check',
      timestamp: '1',
      nonce: 'n',
      body: const [],
    );
    expect(signature, isNotEmpty);
    expect(hmacKey('c2VjcmV0'), utf8.encode('c2VjcmV0'));
  });

  test('manifest canonical string matches the server format', () {
    const update = UpdateInfo(
      available: true,
      updateType: 'recommended',
      version: '1.4.1',
      versionCode: 1004001,
      platform: 'android',
      arch: 'arm64',
      channel: 'stable',
      title: 'Fixes',
      changelog: '- fix',
      minSupportedCode: 1000000,
      force: false,
      package: PackageInfo(
        id: 1,
        fileName: 'demo.apk',
        downloadUrl: 'https://example.com/demo.apk?token=abc',
        size: 12345,
        sha256: 'abc123',
      ),
      manifestSignature: '',
      createdAt: null,
    );

    expect(
      canonicalManifestPayload(appId: 'demo.app', update: update),
      'lodestar-manifest-v1\n'
      'app_id=demo.app\n'
      'version=1.4.1\n'
      'version_code=1004001\n'
      'platform=android\n'
      'arch=arm64\n'
      'channel=stable\n'
      'package_url=https://example.com/demo.apk?token=abc\n'
      'package_size=12345\n'
      'package_sha256=abc123\n'
      'force=false\n'
      'min_supported_code=1000000\n',
    );
  });
}
