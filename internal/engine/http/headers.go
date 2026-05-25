package http

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// Default stealth headers applied to every request. User headers always override.
var defaultHeaders = map[string]string{
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
	"Accept-Language": "en-US,en;q=0.9",
	"Cache-Control":   "no-cache",
	"Pragma":          "no-cache",
}

// MergeStealthHeaders applies generated headers with the given Referer and UA,
// then overlays any user-supplied headers on top (user always wins).
func MergeStealthHeaders(referer, userAgent string, userHeaders map[string]string) http.Header {
	h := http.Header{}
	for k, v := range defaultHeaders {
		h.Set(k, v)
	}
	if referer != "" {
		h.Set("Referer", referer)
	}
	if userAgent != "" {
		h.Set("User-Agent", userAgent)
	}
	for k, v := range userHeaders {
		h.Set(k, v)
	}
	return h
}

// UserAgentForFingerprint returns a UA string matching the given fingerprint.
func UserAgentForFingerprint(fp string) string {
	switch {
	case strings.HasPrefix(fp, "chrome"):
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	case strings.HasPrefix(fp, "safari"):
		return "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"
	default:
		return ""
	}
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	reader := io.Reader(resp.Body)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}
	return io.ReadAll(reader)
}