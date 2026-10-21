using System;
using System.Collections.Generic;
using System.IO;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;

namespace Lodestar.Update;

/// <summary>
/// Signed update checks and verified package downloads for Lodestar.
/// </summary>
/// <example>
/// <code>
/// var client = new LodestarUpdateClient(new LodestarUpdateConfig(
///     "https://lodestar.example.com", "demo.app", secret, publicKey, arch: "amd64"));
/// var response = await client.CheckAsync("1.4.1", 1_004_001);
/// if (response.HasUpdate &amp;&amp; response.Update.IsForce)
/// {
///     var package = await client.DownloadAndVerifyAsync(response.Update, "updates");
/// }
/// </code>
/// </example>
public sealed class LodestarUpdateClient : IDisposable
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    private readonly LodestarUpdateConfig _config;
    private readonly HttpClient _http;
    private readonly bool _ownsHttpClient;

    public LodestarUpdateClient(LodestarUpdateConfig config, HttpClient? httpClient = null)
    {
        _config = config;
        _http = httpClient ?? new HttpClient();
        _ownsHttpClient = httpClient is null;
        if (_http.Timeout == Timeout.InfiniteTimeSpan || _http.Timeout > config.Timeout)
        {
            _http.Timeout = config.Timeout;
        }
    }

    public LodestarUpdateConfig Config => _config;

    /// <summary>Calls <c>POST /api/v1/check</c> with HMAC headers.</summary>
    public async Task<CheckResponse> CheckAsync(
        string version,
        long versionCode,
        string? clientId = null,
        string? installId = null,
        string? deviceId = null,
        string? osVersion = null,
        CancellationToken cancellationToken = default)
    {
        var uri = _config.Resolve("/api/v1/check");
        var body = new Dictionary<string, object?>
        {
            ["app_id"] = _config.AppId,
            ["platform"] = _config.Platform,
            ["arch"] = _config.Arch,
            ["channel"] = _config.Channel,
            ["version"] = version,
            ["version_code"] = versionCode,
        };
        AddIfPresent(body, "client_id", clientId);
        AddIfPresent(body, "install_id", installId);
        AddIfPresent(body, "device_id", deviceId);
        AddIfPresent(body, "os_version", osVersion);

        var payload = Encoding.UTF8.GetBytes(JsonSerializer.Serialize(body, JsonOptions));
        var timestamp = DateTimeOffset.UtcNow.ToUnixTimeSeconds().ToString();
        var nonce = Guid.NewGuid().ToString();
        var signature = LodestarCrypto.SignRequest(
            _config.ClientSecret, "POST", uri.AbsolutePath, timestamp, nonce, payload);

        using var request = new HttpRequestMessage(HttpMethod.Post, uri)
        {
            Content = new ByteArrayContent(payload),
        };
        request.Content.Headers.ContentType = new MediaTypeHeaderValue("application/json");
        request.Headers.TryAddWithoutValidation("Accept", "application/json");
        request.Headers.TryAddWithoutValidation("X-App-Id", _config.AppId);
        request.Headers.TryAddWithoutValidation("X-Timestamp", timestamp);
        request.Headers.TryAddWithoutValidation("X-Nonce", nonce);
        request.Headers.TryAddWithoutValidation("X-Signature", signature);

        using var response = await _http.SendAsync(request, cancellationToken).ConfigureAwait(false);
        var text = await response.Content.ReadAsStringAsync(cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            throw new LodestarHttpException(
                $"Check update failed with HTTP {(int)response.StatusCode}", (int)response.StatusCode, text);
        }
        return JsonSerializer.Deserialize<CheckResponse>(text, JsonOptions)
            ?? throw new LodestarUpdateException("Empty check update response");
    }

    /// <summary>Throws when the manifest signature does not verify.</summary>
    public void VerifyManifest(UpdateInfo update)
    {
        if (!LodestarCrypto.VerifyManifest(_config.Ed25519PublicKey, _config.AppId, update))
        {
            throw new LodestarUpdateException("Update manifest signature verification failed");
        }
    }

    /// <summary>
    /// Downloads the package, verifies its SHA-256 and returns the local file.
    /// The manifest is verified first; a failed hash check deletes the file.
    /// </summary>
    public async Task<DownloadedPackage> DownloadAndVerifyAsync(
        UpdateInfo update,
        string targetDirectory,
        IProgress<(long Received, long Total)>? progress = null,
        CancellationToken cancellationToken = default)
    {
        if (!update.Available)
        {
            throw new LodestarUpdateException("No update package is available");
        }
        if (string.IsNullOrEmpty(update.Package.DownloadUrl))
        {
            throw new LodestarUpdateException("Update package has an empty download URL");
        }
        if (string.IsNullOrEmpty(update.Package.Sha256))
        {
            throw new LodestarUpdateException("Update package has an empty SHA-256");
        }

        VerifyManifest(update);

        Directory.CreateDirectory(targetDirectory);
        var fileName = SafeFileName(update.Package.FileName, $"update_{update.VersionCode}.bin");
        var targetPath = Path.Combine(targetDirectory, fileName);
        var tempPath = targetPath + ".part";

        using var response = await _http
            .GetAsync(update.Package.DownloadUrl, HttpCompletionOption.ResponseHeadersRead, cancellationToken)
            .ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            throw new LodestarHttpException(
                $"Download failed with HTTP {(int)response.StatusCode}", (int)response.StatusCode, null);
        }

        var total = response.Content.Headers.ContentLength ?? update.Package.Size;
        long received = 0;

        try
        {
            using (var source = await response.Content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false))
            using (var destination = File.Create(tempPath))
            using (var sha = IncrementalHash.CreateHash(HashAlgorithmName.SHA256))
            {
                var buffer = new byte[64 * 1024];
                int read;
                while ((read = await source.ReadAsync(buffer, cancellationToken).ConfigureAwait(false)) > 0)
                {
                    await destination.WriteAsync(buffer.AsMemory(0, read), cancellationToken).ConfigureAwait(false);
                    sha.AppendData(buffer, 0, read);
                    received += read;
                    progress?.Report((received, total));
                }

                var actual = LodestarCrypto.ToHex(sha.GetHashAndReset());
                if (!string.Equals(actual, update.Package.Sha256, StringComparison.OrdinalIgnoreCase))
                {
                    throw new LodestarUpdateException(
                        $"SHA-256 mismatch: expected {update.Package.Sha256}, got {actual}");
                }

                destination.Close();
                File.Move(tempPath, targetPath, overwrite: true);
                return new DownloadedPackage(targetPath, fileName, received, actual, update);
            }
        }
        catch
        {
            if (File.Exists(tempPath))
            {
                File.Delete(tempPath);
            }
            throw;
        }
    }

    private static void AddIfPresent(IDictionary<string, object?> body, string key, string? value)
    {
        if (!string.IsNullOrWhiteSpace(value))
        {
            body[key] = value;
        }
    }

    private static string SafeFileName(string name, string fallback)
    {
        if (string.IsNullOrWhiteSpace(name))
        {
            return fallback;
        }
        var cleaned = new StringBuilder(name.Length);
        foreach (var c in name)
        {
            cleaned.Append(char.IsLetterOrDigit(c) || c is '.' or '_' or '-' or '+' ? c : '_');
        }
        var result = cleaned.ToString().Trim();
        return result.Length == 0 || result is "." or ".." ? fallback : result;
    }

    public void Dispose()
    {
        if (_ownsHttpClient)
        {
            _http.Dispose();
        }
    }
}
