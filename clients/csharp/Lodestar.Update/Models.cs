using System.Text.Json.Serialization;

namespace Lodestar.Update;

/// <summary>Response of <c>POST /api/v1/check</c>.</summary>
public sealed class CheckResponse
{
    [JsonPropertyName("request_id")]
    public string RequestId { get; set; } = "";

    [JsonPropertyName("server_time")]
    public long ServerTime { get; set; }

    [JsonPropertyName("legal")]
    public LegalInfo Legal { get; set; } = new();

    [JsonPropertyName("update")]
    public UpdateInfo Update { get; set; } = new();

    public bool IsAllowed => Legal.Allowed;

    public bool HasUpdate => Update.Available;

    /// <summary>A blocked client or a forced update must be applied before use.</summary>
    public bool MustUpdate => !Legal.Allowed || Update.IsForce;
}

public sealed class LegalInfo
{
    [JsonPropertyName("allowed")]
    public bool Allowed { get; set; }

    [JsonPropertyName("code")]
    public string Code { get; set; } = "";

    [JsonPropertyName("message")]
    public string Message { get; set; } = "";
}

public sealed class UpdateInfo
{
    [JsonPropertyName("available")]
    public bool Available { get; set; }

    [JsonPropertyName("update_type")]
    public string UpdateType { get; set; } = "none";

    [JsonPropertyName("version")]
    public string Version { get; set; } = "";

    [JsonPropertyName("version_code")]
    public long VersionCode { get; set; }

    [JsonPropertyName("platform")]
    public string Platform { get; set; } = "";

    [JsonPropertyName("arch")]
    public string Arch { get; set; } = "";

    [JsonPropertyName("channel")]
    public string Channel { get; set; } = "";

    [JsonPropertyName("title")]
    public string Title { get; set; } = "";

    [JsonPropertyName("changelog")]
    public string Changelog { get; set; } = "";

    [JsonPropertyName("min_supported_code")]
    public long MinSupportedCode { get; set; }

    [JsonPropertyName("force")]
    public bool Force { get; set; }

    [JsonPropertyName("package")]
    public PackageInfo Package { get; set; } = new();

    [JsonPropertyName("manifest_signature")]
    public string ManifestSignature { get; set; } = "";

    [JsonPropertyName("created_at")]
    public DateTimeOffset? CreatedAt { get; set; }

    public bool IsForce => Force || UpdateType == "force";

    public bool IsRecommended => UpdateType == "recommended" && !IsForce;
}

public sealed class PackageInfo
{
    [JsonPropertyName("id")]
    public long Id { get; set; }

    [JsonPropertyName("file_name")]
    public string FileName { get; set; } = "";

    [JsonPropertyName("download_url")]
    public string DownloadUrl { get; set; } = "";

    [JsonPropertyName("size")]
    public long Size { get; set; }

    [JsonPropertyName("sha256")]
    public string Sha256 { get; set; } = "";
}

/// <summary>A verified package on disk.</summary>
public sealed class DownloadedPackage
{
    public DownloadedPackage(string filePath, string fileName, long size, string sha256, UpdateInfo update)
    {
        FilePath = filePath;
        FileName = fileName;
        Size = size;
        Sha256 = sha256;
        Update = update;
    }

    public string FilePath { get; }

    public string FileName { get; }

    public long Size { get; }

    public string Sha256 { get; }

    public UpdateInfo Update { get; }
}
