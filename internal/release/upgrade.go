package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Options configures Latest and Upgrade. Everything is injected — platform,
// paths, server, keys — so tests run the real flow against a fake release,
// and only main.go supplies runtime.GOOS and friends.
type Options struct {
	APIURL       string       // releases/latest endpoint; "" means DefaultAPI
	Client       *http.Client // nil means a client with a sane timeout
	GOOS, GOARCH string       // the platform to install a build for
	Current      Version      // the installed binary's version
	Target       string       // path of the binary to replace
	Trusted      []string     // release-signing keys (authorized_keys lines)
	Force        bool         // reinstall even when Current is the latest
	// RunVersion runs a binary with --version and returns its output: the
	// smoke test that the new binary starts before it replaces the old one.
	// nil means execute it.
	RunVersion func(ctx context.Context, path string) (string, error)
}

// Result is what Upgrade did.
type Result struct {
	From, To Version
	Upgraded bool // false when already on the latest release
}

func (o Options) client() *http.Client {
	if o.Client != nil {
		return o.Client
	}
	return defaultClient()
}

func (o Options) api() string {
	if o.APIURL != "" {
		return o.APIURL
	}
	return DefaultAPI
}

// Latest reports the newest published release and whether it is newer than
// o.Current. Only release metadata is downloaded.
func Latest(ctx context.Context, o Options) (Version, bool, error) {
	rel, err := fetchLatest(ctx, o.client(), o.api())
	if err != nil {
		return Version{}, false, err
	}
	v, err := ParseVersion(rel.Tag)
	if err != nil {
		return Version{}, false, fmt.Errorf("latest release tag: %w", err)
	}
	return v, o.Current.Less(v), nil
}

// Upgrade replaces o.Target with the newest signed release. Everything is
// verified before o.Target is touched, in this order: the release is not a
// downgrade; checksums.txt carries a valid signature from a trusted key in
// Namespace; the signed checksums list this platform's archive by its exact
// versioned name (binding the version to what was signed); the archive's
// SHA-256 matches; the archive holds the binary; and the new binary runs and
// reports the release version. Any failure returns an error with o.Target
// exactly as it was.
func Upgrade(ctx context.Context, o Options) (Result, error) {
	c := o.client()
	rel, err := fetchLatest(ctx, c, o.api())
	if err != nil {
		return Result{}, err
	}
	to, err := ParseVersion(rel.Tag)
	if err != nil {
		return Result{}, fmt.Errorf("latest release tag: %w", err)
	}
	res := Result{From: o.Current, To: to}
	switch {
	case to.Less(o.Current):
		return res, fmt.Errorf("the latest release (%s) is older than this strata (%s) — refusing to downgrade", to, o.Current)
	case to == o.Current && !o.Force:
		return res, nil
	}

	bin, ext := "strata", "tar.gz"
	if o.GOOS == "windows" {
		bin, ext = "strata.exe", "zip"
	}
	archiveName := fmt.Sprintf("strata_%s_%s_%s.%s", to, o.GOOS, o.GOARCH, ext)
	sumsURL, haveSums := rel.assetURL("checksums.txt")
	sigURL, haveSig := rel.assetURL("checksums.txt.sig")
	if !haveSums || !haveSig {
		return res, fmt.Errorf("release %s isn't signed (no checksums.txt.sig) — refusing to install it", rel.Tag)
	}
	archiveURL, ok := rel.assetURL(archiveName)
	if !ok {
		return res, fmt.Errorf("release %s has no build for %s/%s (%s)", rel.Tag, o.GOOS, o.GOARCH, archiveName)
	}

	// Nothing downloaded below is trusted until the checksum list verifies.
	sums, err := get(ctx, c, sumsURL, maxMeta, "")
	if err != nil {
		return res, err
	}
	sig, err := get(ctx, c, sigURL, maxMeta, "")
	if err != nil {
		return res, err
	}
	if err := VerifySSHSig(sums, sig, Namespace, o.Trusted); err != nil {
		return res, fmt.Errorf("release %s: signature check failed: %w", rel.Tag, err)
	}
	want, ok := checksumFor(sums, archiveName)
	if !ok {
		return res, fmt.Errorf("release %s: signed checksums don't list %s", rel.Tag, archiveName)
	}

	archive, err := get(ctx, c, archiveURL, maxArchive, "")
	if err != nil {
		return res, err
	}
	if got := sha256Hex(archive); got != want {
		return res, fmt.Errorf("%s: checksum mismatch (downloaded %s, signed %s) — refusing to install", archiveName, got, want)
	}
	data, err := ExtractBinary(archive, ext, bin)
	if err != nil {
		return res, fmt.Errorf("%s: %w", archiveName, err)
	}

	tmp, err := writeTemp(o.Target, data)
	if err != nil {
		return res, err
	}
	defer os.Remove(tmp) // no-op once installed
	out, err := o.runVersion(ctx, tmp)
	if err != nil {
		return res, fmt.Errorf("the new binary won't run: %w", err)
	}
	if !reportsVersion(out, to) {
		return res, fmt.Errorf("the new binary reports %q, expected version %s — refusing to install", strings.TrimSpace(out), to)
	}
	if err := install(tmp, o.Target, o.GOOS); err != nil {
		return res, err
	}
	res.Upgraded = true
	return res, nil
}

func (o Options) runVersion(ctx context.Context, path string) (string, error) {
	if o.RunVersion != nil {
		return o.RunVersion(ctx, path)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	return string(out), err
}

// reportsVersion reports whether `strata --version` output names exactly v
// ("strata version 2026.9.1"). A substring check would accept 2026.9.10.
func reportsVersion(out string, v Version) bool {
	f := strings.Fields(out)
	return len(f) >= 2 && f[len(f)-2] == "version" && f[len(f)-1] == v.String()
}

// checksumFor finds name's hash in a checksums.txt ("<sha256>  <name>").
func checksumFor(sums []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == name {
			return f[0], true
		}
	}
	return "", false
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
