class LodestarUpdateException implements Exception {
  const LodestarUpdateException(this.message, {this.cause});

  final String message;
  final Object? cause;

  @override
  String toString() {
    if (cause == null) return 'LodestarUpdateException: $message';
    return 'LodestarUpdateException: $message; cause=$cause';
  }
}

class LodestarHttpException extends LodestarUpdateException {
  const LodestarHttpException(
    super.message, {
    required this.statusCode,
    this.body,
    super.cause,
  });

  final int statusCode;
  final String? body;

  @override
  String toString() =>
      'LodestarHttpException: $message; statusCode=$statusCode; body=$body';
}

class LodestarUpdateCancelledException extends LodestarUpdateException {
  const LodestarUpdateCancelledException()
      : super('Update package download was cancelled.');

  @override
  String toString() => 'LodestarUpdateCancelledException: $message';
}
