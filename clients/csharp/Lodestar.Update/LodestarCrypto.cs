using System;
using System.Security.Cryptography;
using System.Text;
using Org.BouncyCastle.Crypto.Parameters;
using Org.BouncyCastle.Crypto.Signers;

namespace Lodestar.Update;

/// <summary>
/// Signing helpers. The request signature is
/// <c>base64(HMAC-SHA256(secret, METHOD\nPATH\nTIMESTAMP\nNONCE\nsha256(body)))</c>
/// and the key is the raw bytes of <c>client_secret</c> — the server never
/// base64-decodes it.
/// </summary>
public static class LodestarCrypto
{
    public static byte[] HmacKey(string secret) => Encoding.UTF8.GetBytes(secret);

    public static string BodySha256Hex(byte[] body)
    {
        using var sha = SHA256.Create();
        return ToHex(sha.ComputeHash(body));
    }

    public static string SigningPayload(string method, string path, string timestamp, string nonce, byte[] body)
    {
        return string.Join(
            "\n",
            method.ToUpperInvariant(),
            path,
            timestamp,
            nonce,
            BodySha256Hex(body));
    }

    public static string SignRequest(
        string secret, string method, string path, string timestamp, string nonce, byte[] body)
    {
        using var hmac = new HMACSHA256(HmacKey(secret));
        var digest = hmac.ComputeHash(Encoding.UTF8.GetBytes(SigningPayload(method, path, timestamp, nonce, body)));
        return Convert.ToBase64String(digest);
    }

    /// <summary>The exact byte sequence the server signs with its Ed25519 key.</summary>
    public static string CanonicalManifest(string appId, UpdateInfo update)
    {
        return "lodestar-manifest-v1\n"
            + "app_id=" + appId + "\n"
            + "version=" + update.Version + "\n"
            + "version_code=" + update.VersionCode + "\n"
            + "platform=" + update.Platform + "\n"
            + "arch=" + update.Arch + "\n"
            + "channel=" + update.Channel + "\n"
            + "package_url=" + update.Package.DownloadUrl + "\n"
            + "package_size=" + update.Package.Size + "\n"
            + "package_sha256=" + update.Package.Sha256 + "\n"
            + "force=" + (update.Force ? "true" : "false") + "\n"
            + "min_supported_code=" + update.MinSupportedCode + "\n";
    }

    /// <summary>
    /// Verifies the manifest signature. Returns true when there is no update to
    /// verify, false when the signature is missing or wrong.
    /// </summary>
    public static bool VerifyManifest(string publicKeyBase64, string appId, UpdateInfo update)
    {
        if (!update.Available)
        {
            return true;
        }
        if (string.IsNullOrEmpty(update.ManifestSignature))
        {
            return false;
        }

        var publicKey = Convert.FromBase64String(publicKeyBase64.Trim());
        var signature = Convert.FromBase64String(update.ManifestSignature);
        if (publicKey.Length != 32)
        {
            throw new LodestarUpdateException($"Invalid Ed25519 public key length: {publicKey.Length}");
        }
        if (signature.Length != 64)
        {
            throw new LodestarUpdateException($"Invalid Ed25519 signature length: {signature.Length}");
        }

        var signer = new Ed25519Signer();
        signer.Init(false, new Ed25519PublicKeyParameters(publicKey, 0));
        var payload = Encoding.UTF8.GetBytes(CanonicalManifest(appId, update));
        signer.BlockUpdate(payload, 0, payload.Length);
        return signer.VerifySignature(signature);
    }

    public static string ToHex(byte[] bytes)
    {
        var builder = new StringBuilder(bytes.Length * 2);
        foreach (var b in bytes)
        {
            builder.Append(b.ToString("x2"));
        }
        return builder.ToString();
    }
}
