import 'package:shared_preferences/shared_preferences.dart';
import 'package:uuid/uuid.dart';

class LodestarIdentity {
  static const String defaultInstallIdKey = 'lodestar_update_install_id';

  static Future<String> getOrCreateInstallId({
    String key = defaultInstallIdKey,
  }) async {
    final prefs = await SharedPreferences.getInstance();
    final existing = prefs.getString(key);
    if (existing != null && existing.isNotEmpty) return existing;
    final id = const Uuid().v4();
    await prefs.setString(key, id);
    return id;
  }
}
