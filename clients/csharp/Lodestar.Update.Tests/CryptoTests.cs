using System.Text;
using Lodestar.Update;
using Org.BouncyCastle.Crypto.Generators;
using Org.BouncyCastle.Crypto.Parameters;
using Org.BouncyCastle.Crypto.Signers;
using Org.BouncyCastle.Security;
using Xunit;

namespace Lodestar.Update.Tests;

public class CryptoTests
{
    private const string Body =
        "{\"app_id\":\"demo.app\",\"arch\":\"amd64\",\"channel\":\"stable\",\"platform\":\"linux\",\"version\":\"1.0.0\",\"version_code\":1000000}";

    [Fact]
    public void SigningPayloadMatchesServer()
    {
        var payload = LodestarCrypto.SigningPayload(
            "post", "/api/v1/check", "1700000000", "nonce-1", Encoding.UTF8.GetBytes(Body));
        Assert.Equal(
            "POST\n/api/v1/check\n1700000000\nnonce-1\n" +
            "8a88dcf3efd954856da26800fd6c395508f4c6b5d891ff1586ee8578253009b9",
            payload);
    }

    [Fact]
    public void HmacUsesRawSecret()
    {
        var signature = LodestarCrypto.SignRequest(
            "s3cr3t-key", "POST", "/api/v1/check", "1700000000", "nonce-1", Encoding.UTF8.GetBytes(Body));
        Assert.Equal("ZwVMKnpQgsB2YjLgGYpogIGq4DeR63ygFLWAKQywgzQ=", signature);
    }

    [Fact]
    public void Base64LookingSecretIsNotDecoded()
    {
        Assert.Equal(
            Encoding.UTF8.GetBytes("c2VjcmV0"),
            LodestarCrypto.HmacKey("c2VjcmV0"));
    }

    [Fact]
    public void CanonicalManifestMatchesServer()
    {
        var update = SampleUpdate();
        Assert.Equal(
            "lodestar-manifest-v1\n" +
            "app_id=demo.app\n" +
            "version=1.4.1\n" +
            "version_code=1004001\n" +
            "platform=android\n" +
            "arch=arm64\n" +
            "channel=stable\n" +
            "package_url=https://example.com/demo.apk?token=abc\n" +
            "package_size=12345\n" +
            "package_sha256=abc123\n" +
            "force=false\n" +
            "min_supported_code=1000000\n",
            LodestarCrypto.CanonicalManifest("demo.app", update));
    }

    [Fact]
    public void ManifestVerificationAcceptsRealSignature()
    {
        var generator = new Ed25519KeyPairGenerator();
        generator.Init(new Ed25519KeyGenerationParameters(new SecureRandom()));
        var pair = generator.GenerateKeyPair();
        var publicKey = (Ed25519PublicKeyParameters)pair.Public;
        var privateKey = (Ed25519PrivateKeyParameters)pair.Private;

        var update = SampleUpdate();
        var payload = Encoding.UTF8.GetBytes(LodestarCrypto.CanonicalManifest("demo.app", update));
        var signer = new Ed25519Signer();
        signer.Init(true, privateKey);
        signer.BlockUpdate(payload, 0, payload.Length);
        update.ManifestSignature = Convert.ToBase64String(signer.GenerateSignature());

        var publicKeyBase64 = Convert.ToBase64String(publicKey.GetEncoded());
        Assert.True(LodestarCrypto.VerifyManifest(publicKeyBase64, "demo.app", update));

        // A tampered version must invalidate the signature.
        update.Version = "9.9.9";
        Assert.False(LodestarCrypto.VerifyManifest(publicKeyBase64, "demo.app", update));
    }

    [Fact]
    public void MissingSignatureIsRejected()
    {
        var update = SampleUpdate();
        update.ManifestSignature = "";
        Assert.False(LodestarCrypto.VerifyManifest(Convert.ToBase64String(new byte[32]), "demo.app", update));
    }

    private static UpdateInfo SampleUpdate() => new()
    {
        Available = true,
        UpdateType = "recommended",
        Version = "1.4.1",
        VersionCode = 1004001,
        Platform = "android",
        Arch = "arm64",
        Channel = "stable",
        MinSupportedCode = 1000000,
        Force = false,
        Package = new PackageInfo
        {
            Id = 1,
            FileName = "demo.apk",
            DownloadUrl = "https://example.com/demo.apk?token=abc",
            Size = 12345,
            Sha256 = "abc123",
        },
    };
}
