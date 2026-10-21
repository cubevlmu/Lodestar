int versionCodeFromVersion(String version) {
  var v = version.trim();

  if (v.startsWith('v') || v.startsWith('V')) {
    v = v.substring(1);
  }

  final suffixIndex = v.indexOf(RegExp(r'[+-]'));
  if (suffixIndex >= 0) {
    v = v.substring(0, suffixIndex);
  }

  final parts = v.split('.');
  if (parts.isEmpty || parts.length > 3) {
    throw FormatException('Invalid version: $version');
  }

  final nums = [0, 0, 0];

  for (var i = 0; i < parts.length; i++) {
    final part = parts[i];

    if (part.isEmpty) {
      throw FormatException('Invalid version: $version');
    }

    final n = int.tryParse(part);
    if (n == null || n < 0) {
      throw FormatException('Invalid version: $version');
    }

    nums[i] = n;
  }

  return nums[0] * 1000000 + nums[1] * 1000 + nums[2];
}
