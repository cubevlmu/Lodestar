using Lodestar.Update;

// Minimal command line check: verify the manifest, optionally download.
string? Arg(string name, string? fallback = null)
{
    for (var i = 0; i < args.Length - 1; i++)
    {
        if (args[i] == name)
        {
            return args[i + 1];
        }
    }
    return fallback;
}

var url = Arg("--url", "http://127.0.0.1:8080")!;
var appId = Arg("--app");
var secret = Arg("--secret");
var publicKey = Arg("--pub");
var version = Arg("--version", "1.0.0")!;
var code = long.Parse(Arg("--code", "1000000")!);
var platform = Arg("--platform");
var arch = Arg("--arch", "universal")!;
var channel = Arg("--channel", "stable")!;
var downloadDir = Arg("--download");

if (appId is null || secret is null || publicKey is null)
{
    Console.Error.WriteLine(
        "usage: Lodestar.Update.Example --app <app_id> --secret <client_secret> --pub <ed25519_public_key> " +
        "[--url ...] [--version ...] [--code ...] [--platform ...] [--arch ...] [--channel ...] [--download <dir>]");
    return 2;
}

var config = new LodestarUpdateConfig(url, appId, secret, publicKey, platform, arch, channel);
using var client = new LodestarUpdateClient(config);

var response = await client.CheckAsync(version, code, "csharp-example");
Console.WriteLine($"legal.allowed = {response.Legal.Allowed}");
Console.WriteLine($"update.available = {response.HasUpdate}");
if (!response.HasUpdate)
{
    Console.WriteLine("already up to date");
    return 0;
}

var update = response.Update;
Console.WriteLine($"version = {update.Version} ({update.VersionCode})");
Console.WriteLine($"force = {update.IsForce}, type = {update.UpdateType}");
Console.WriteLine($"package = {update.Package.FileName} ({update.Package.Size} bytes)");

client.VerifyManifest(update);
Console.WriteLine("manifest signature = ok");

if (downloadDir is not null)
{
    var progress = new Progress<(long Received, long Total)>(p => Console.WriteLine($"  {p.Received}/{p.Total}"));
    var package = await client.DownloadAndVerifyAsync(update, downloadDir, progress);
    Console.WriteLine($"downloaded = {package.FilePath} ({package.Sha256})");
}

return 0;
