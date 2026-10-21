using System;

namespace Lodestar.Update;

/// <summary>Configuration for <see cref="LodestarUpdateClient"/>.</summary>
public sealed class LodestarUpdateConfig
{
    public LodestarUpdateConfig(
        string baseUrl,
        string appId,
        string clientSecret,
        string ed25519PublicKey,
        string? platform = null,
        string arch = "universal",
        string channel = "stable",
        TimeSpan? timeout = null)
    {
        if (string.IsNullOrWhiteSpace(baseUrl))
        {
            throw new ArgumentException("baseUrl is required", nameof(baseUrl));
        }
        if (string.IsNullOrWhiteSpace(appId))
        {
            throw new ArgumentException("appId is required", nameof(appId));
        }
        if (string.IsNullOrEmpty(clientSecret))
        {
            throw new ArgumentException("clientSecret is required", nameof(clientSecret));
        }
        if (string.IsNullOrWhiteSpace(ed25519PublicKey))
        {
            throw new ArgumentException("ed25519PublicKey is required", nameof(ed25519PublicKey));
        }

        BaseUrl = baseUrl.TrimEnd('/');
        AppId = appId.Trim();
        ClientSecret = clientSecret;
        Ed25519PublicKey = ed25519PublicKey.Trim();
        Platform = string.IsNullOrWhiteSpace(platform) ? DetectPlatform() : platform!;
        Arch = arch;
        Channel = channel;
        Timeout = timeout ?? TimeSpan.FromSeconds(30);
    }

    public string BaseUrl { get; }

    public string AppId { get; }

    /// <summary>Raw secret used for HMAC signing. Not a hard security boundary.</summary>
    public string ClientSecret { get; }

    /// <summary>Base64 Ed25519 public key from the project page.</summary>
    public string Ed25519PublicKey { get; }

    public string Platform { get; }

    public string Arch { get; }

    public string Channel { get; }

    public TimeSpan Timeout { get; }

    public Uri Resolve(string path)
    {
        var normalized = path.StartsWith("/", StringComparison.Ordinal) ? path : "/" + path;
        return new Uri(BaseUrl + normalized);
    }

    /// <summary>Maps the runtime OS to the platform token Lodestar expects.</summary>
    public static string DetectPlatform()
    {
        if (OperatingSystem.IsWindows())
        {
            return "windows";
        }
        if (OperatingSystem.IsMacOS() || OperatingSystem.IsMacCatalyst())
        {
            return "darwin";
        }
        if (OperatingSystem.IsLinux())
        {
            return "linux";
        }
        if (OperatingSystem.IsAndroid())
        {
            return "android";
        }
        if (OperatingSystem.IsIOS())
        {
            return "ios";
        }
        return "unknown";
    }
}
