package release

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultAPI is GitHub's endpoint for strata's newest published release.
const DefaultAPI = "https://api.github.com/repos/D1srupt3d/strata/releases/latest"

// Download caps. Release metadata and checksums are tiny; archives are a few
// MB. A response over its cap is an error, never truncated.
const (
	maxMeta    = 1 << 20
	maxArchive = 250 << 20
)

// latestRelease is the part of GitHub's release JSON strata reads.
type latestRelease struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r latestRelease) assetURL(name string) (string, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL, true
		}
	}
	return "", false
}

func fetchLatest(ctx context.Context, c *http.Client, api string) (latestRelease, error) {
	body, err := get(ctx, c, api, maxMeta, "application/vnd.github+json")
	if err != nil {
		return latestRelease{}, fmt.Errorf("checking for releases: %w", err)
	}
	var r latestRelease
	if err := json.Unmarshal(body, &r); err != nil {
		return latestRelease{}, fmt.Errorf("reading release info from %s: %w", api, err)
	}
	if r.Tag == "" {
		return latestRelease{}, fmt.Errorf("no published release found at %s", api)
	}
	return r, nil
}

// get fetches url, failing on any non-200 status or a body over max bytes.
func get(ctx context.Context, c *http.Client, url string, max int64, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("GET %s: response over %d bytes", url, max)
	}
	return b, nil
}

func defaultClient() *http.Client { return &http.Client{Timeout: 5 * time.Minute} }
