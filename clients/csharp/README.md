# Lodestar.Update (C#)

.NET 8 client for [Lodestar](https://github.com/cubevlmu/lodestar). Ed25519
verification uses BouncyCastle; everything else is in the framework.

## Install

Download `Lodestar.Update.<version>.nupkg` from the
[releases page](https://github.com/cubevlmu/lodestar/releases), drop it in a
folder, and add that folder as a local feed:

```bash
mkdir -p nuget && cp ~/Downloads/Lodestar.Update.0.1.0.nupkg nuget/
dotnet nuget add source ./nuget --name lodestar
dotnet add package Lodestar.Update
```

## Use

```csharp
using Lodestar.Update;

var config = new LodestarUpdateConfig(
    baseUrl: "https://lodestar.example.com",
    appId: "demo.app",
    clientSecret: secret,
    ed25519PublicKey: publicKey,
    arch: "amd64");

using var client = new LodestarUpdateClient(config);
var response = await client.CheckAsync("1.4.1", 1_004_001);

if (response.HasUpdate)
{
    client.VerifyManifest(response.Update);                       // Ed25519
    var package = await client.DownloadAndVerifyAsync(response.Update, "updates");
    Console.WriteLine($"{package.FilePath} {package.Sha256}");
}
```

`response.MustUpdate` is true when the client is blocked by policy or the update
is forced.

## Build and test

```bash
dotnet build Lodestar.Update
dotnet test Lodestar.Update.Tests
dotnet pack Lodestar.Update -c Release
```
