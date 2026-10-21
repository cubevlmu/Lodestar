// Example: check for updates from a Flutter / Dart client.
//
// Add the dependencies first:
//   dart pub add crypto
//
// Run:
//   dart run examples/client/dart/check_update.dart \
//     --url=https://lodestar.example.com --app=demo-app --secret=$CLIENT_SECRET \
//     --platform=android --version=1.4.1 --code=1004001
//
// The signing rules match internal/security.SignRequestHMAC:
//   payload   = METHOD \n PATH \n TIMESTAMP \n NONCE \n hex(sha256(body))
//   signature = base64(HMAC-SHA256(utf8(client_secret), payload))
//
// Manifest signature verification needs an Ed25519 implementation
// (e.g. package:pointycastle or package:ed25519_edwards); the canonical payload
// format is documented in README.md under "客户端自动更新".

import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:crypto/crypto.dart';

Future<void> main(List<String> argv) async {
  final args = _parseArgs(argv);
  final baseUrl = (args['url'] ?? 'http://127.0.0.1:8080').replaceAll(RegExp(r'/$'), '');
  final appId = args['app'];
  final secret = args['secret'];
  if (appId == null || secret == null) {
    stderr.writeln('usage: check_update --app=<app_id> --secret=<client_secret> [--url=] [--platform=] [--arch=] [--channel=] [--version=] [--code=]');
    exit(2);
  }

  final body = jsonEncode({
    'app_id': appId,
    'platform': args['platform'] ?? 'android',
    'arch': args['arch'] ?? 'arm64',
    'channel': args['channel'] ?? 'stable',
    'version': args['version'] ?? '1.0.0',
    'version_code': int.tryParse(args['code'] ?? '') ?? 1000000,
    'client_id': 'dart-example',
  });

  const path = '/api/v1/check';
  final timestamp = (DateTime.now().millisecondsSinceEpoch ~/ 1000).toString();
  final nonce = _randomHex(16);
  final signature = _signRequest(secret, 'POST', path, timestamp, nonce, body);

  final client = HttpClient();
  try {
    final request = await client.postUrl(Uri.parse('$baseUrl$path'));
    request.headers.contentType = ContentType.json;
    request.headers.set('X-App-Id', appId);
    request.headers.set('X-Timestamp', timestamp);
    request.headers.set('X-Nonce', nonce);
    request.headers.set('X-Signature', signature);
    request.add(utf8.encode(body));

    final response = await request.close();
    final text = await response.transform(utf8.decoder).join();
    if (response.statusCode != 200) {
      stderr.writeln('check failed: ${response.statusCode} $text');
      exit(1);
    }

    final data = jsonDecode(text) as Map<String, dynamic>;
    final legal = data['legal'] as Map<String, dynamic>? ?? const {};
    print('legal    allowed=${legal['allowed']} code=${legal['code']}');

    final update = data['update'] as Map<String, dynamic>?;
    if (update == null || update['available'] != true) {
      print('update   up to date');
      return;
    }

    final pkg = update['package'] as Map<String, dynamic>? ?? const {};
    print('update   ${update['version']} (${update['version_code']}) type=${update['update_type']} force=${update['force']}');
    print('changelog ${update['changelog']}');
    print('package  ${pkg['file_name']} ${pkg['size']} bytes');
    print('download ${pkg['download_url']}');
  } finally {
    client.close(force: true);
  }
}

String _signRequest(String secret, String method, String path, String timestamp, String nonce, String body) {
  final bodyHash = sha256.convert(utf8.encode(body)).toString();
  final payload = '${method.toUpperCase()}\n$path\n$timestamp\n$nonce\n$bodyHash';
  final mac = Hmac(sha256, utf8.encode(secret)).convert(utf8.encode(payload));
  return base64Encode(mac.bytes);
}

String _randomHex(int bytes) {
  final random = Random.secure();
  final buffer = List<int>.generate(bytes, (_) => random.nextInt(256));
  return buffer.map((b) => b.toRadixString(16).padLeft(2, '0')).join();
}

Map<String, String> _parseArgs(List<String> argv) {
  final result = <String, String>{};
  for (final arg in argv) {
    if (!arg.startsWith('--')) continue;
    final index = arg.indexOf('=');
    if (index < 0) continue;
    result[arg.substring(2, index)] = arg.substring(index + 1);
  }
  return result;
}
