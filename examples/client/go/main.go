// Example: check for updates from a Go client, then verify the manifest signature.
//
//	go run ./examples/client/go \
//	  -url https://lodestar.example.com -app demo-app \
//	  -secret "$CLIENT_SECRET" -pub "$ED25519_PUBLIC_KEY" \
//	  -platform windows -arch amd64 -version 1.4.1 -code 1004001
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type checkRequest struct {
	AppID       string `json:"app_id"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	Channel     string `json:"channel"`
	Version     string `json:"version"`
	VersionCode int64  `json:"version_code"`
	ClientID    string `json:"client_id,omitempty"`
	InstallID   string `json:"install_id,omitempty"`
}

type legalInfo struct {
	Allowed bool   `json:"allowed"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type packageInfo struct {
	ID          uint   `json:"id"`
	FileName    string `json:"file_name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type updateInfo struct {
	Available         bool        `json:"available"`
	UpdateType        string      `json:"update_type"`
	Version           string      `json:"version"`
	VersionCode       int64       `json:"version_code"`
	Platform          string      `json:"platform"`
	Arch              string      `json:"arch"`
	Channel           string      `json:"channel"`
	Title             string      `json:"title"`
	Changelog         string      `json:"changelog"`
	MinSupportedCode  int64       `json:"min_supported_code"`
	Force             bool        `json:"force"`
	Package           packageInfo `json:"package"`
	ManifestSignature string      `json:"manifest_signature"`
}

type checkResponse struct {
	RequestID  string      `json:"request_id"`
	ServerTime int64       `json:"server_time"`
	Legal      legalInfo   `json:"legal"`
	Update     *updateInfo `json:"update"`
}

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:8080", "Lodestar base URL")
	appID := flag.String("app", "", "app_id")
	secret := flag.String("secret", "", "client_secret")
	publicKey := flag.String("pub", "", "Ed25519 public key (base64) for manifest verification")
	platform := flag.String("platform", "linux", "platform")
	arch := flag.String("arch", "amd64", "arch")
	channel := flag.String("channel", "stable", "channel")
	version := flag.String("version", "1.0.0", "current version")
	code := flag.Int64("code", 1000000, "current version code")
	flag.Parse()

	if *appID == "" || *secret == "" {
		fmt.Fprintln(os.Stderr, "usage: check_update -app <app_id> -secret <client_secret> [flags]")
		os.Exit(2)
	}

	body, err := json.Marshal(checkRequest{
		AppID:       *appID,
		Platform:    *platform,
		Arch:        *arch,
		Channel:     *channel,
		Version:     *version,
		VersionCode: *code,
		ClientID:    "example-client",
	})
	must(err)

	path := "/api/v1/check"
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonce := randomHex(16)
	signature := signRequest(*secret, http.MethodPost, path, timestamp, nonce, body)

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(*baseURL, "/")+path, bytes.NewReader(body))
	must(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Id", *appID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", signature)

	resp, err := http.DefaultClient.Do(req)
	must(err)
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	must(err)
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "check failed: %s %s\n", resp.Status, payload)
		os.Exit(1)
	}

	var result checkResponse
	must(json.Unmarshal(payload, &result))

	fmt.Printf("request  %s\n", result.RequestID)
	fmt.Printf("legal    allowed=%v code=%s message=%s\n", result.Legal.Allowed, result.Legal.Code, result.Legal.Message)
	if result.Update == nil || !result.Update.Available {
		fmt.Println("update   up to date")
		return
	}

	up := result.Update
	fmt.Printf("update   %s (%d) type=%s force=%v\n", up.Version, up.VersionCode, up.UpdateType, up.Force)
	fmt.Printf("package  %s %d bytes sha256=%s\n", up.Package.FileName, up.Package.Size, up.Package.SHA256)
	fmt.Printf("download %s\n", up.Package.DownloadURL)

	if *publicKey == "" {
		fmt.Println("verify   skipped (no -pub)")
		return
	}
	ok := verifyManifest(*publicKey, *appID, up)
	fmt.Printf("verify   manifest_signature=%v\n", ok)
	if !ok {
		os.Exit(1)
	}
}

// signRequest mirrors internal/security.SignRequestHMAC.
func signRequest(secret, method, path, timestamp, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	payload := strings.ToUpper(method) + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// canonicalManifest mirrors internal/security.CanonicalManifestPayload.
func canonicalManifest(appID string, up *updateInfo) string {
	return fmt.Sprintf(
		"lodestar-manifest-v1\napp_id=%s\nversion=%s\nversion_code=%d\nplatform=%s\narch=%s\nchannel=%s\npackage_url=%s\npackage_size=%d\npackage_sha256=%s\nforce=%t\nmin_supported_code=%d\n",
		appID,
		up.Version,
		up.VersionCode,
		up.Platform,
		up.Arch,
		up.Channel,
		up.Package.DownloadURL,
		up.Package.Size,
		up.Package.SHA256,
		up.Force,
		up.MinSupportedCode,
	)
}

func verifyManifest(publicKeyBase64, appID string, up *updateInfo) bool {
	pub, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(up.ManifestSignature)
	if err != nil {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), []byte(canonicalManifest(appID, up)), sig)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
