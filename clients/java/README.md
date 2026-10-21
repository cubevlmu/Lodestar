# lodestar-update (Java)

Java 17+ client for [Lodestar](https://github.com/cubevlmu/lodestar). No runtime
dependencies: HMAC and Ed25519 come from the JDK, HTTP from
`java.net.http.HttpClient`, and JSON is parsed by a ~200 line internal reader.

## Install

Download `lodestar-update-<version>.jar` (plus the sources/javadoc jars) from
the [releases page](https://github.com/cubevlmu/lodestar/releases) and install
it into your local Maven repository:

```bash
mvn install:install-file \
  -Dfile=lodestar-update-0.1.0.jar \
  -DgroupId=com.cubevlmu -DartifactId=lodestar-update \
  -Dversion=0.1.0 -Dpackaging=jar
```

Then:

```xml
<dependency>
  <groupId>com.cubevlmu</groupId>
  <artifactId>lodestar-update</artifactId>
  <version>0.1.0</version>
</dependency>
```

Gradle users can point at the jar directly:

```kotlin
implementation(files("libs/lodestar-update-0.1.0.jar"))
```

## Use

```java
var config = LodestarUpdateConfig.builder()
    .baseUrl("https://lodestar.example.com")
    .appId("demo.app")
    .clientSecret(secret)
    .ed25519PublicKey(publicKey)
    .arch("amd64")
    .build();

try (var client = new LodestarUpdateClient(config)) {
    var response = client.check("1.4.1", 1_004_001, null);
    if (response.hasUpdate()) {
        client.verifyManifest(response.update());          // Ed25519
        var pkg = client.downloadAndVerify(response.update(), Path.of("updates"));
        System.out.println(pkg.file() + " " + pkg.sha256());
    }
}
```

`response.mustUpdate()` is true when the client is blocked by policy or the
update is forced.

## Build

```bash
mvn package                 # jar + sources + javadoc
javac --release 17 -d out $(find src -name '*.java')
java -cp out com.cubevlmu.lodestar.update.SelfTest
```

The self test has no JUnit dependency so it also runs with a bare JDK.
