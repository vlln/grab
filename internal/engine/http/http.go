package http

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/vlln/grab/internal/engine"
	"github.com/vlln/grab/internal/fingerprint"
)

type HTTPEngine struct {
	Profile    fingerprint.Profile
	maxRetries int
	retryDelay time.Duration
}

func NewHTTPEngine(profile fingerprint.Profile, maxRetries int, retryDelay time.Duration) *HTTPEngine {
	return &HTTPEngine{
		Profile:    profile,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

func (e *HTTPEngine) Name() string { return "http" }

func (e *HTTPEngine) Fetch(ctx context.Context, req engine.FetchRequest) (*engine.FetchResult, error) {
	var lastErr error
	for attempt := 0; attempt < e.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(e.retryDelay):
			}
		}
		result, err := e.fetchOnce(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		if result.StatusCode == 403 || result.StatusCode == 503 {
			lastErr = fmt.Errorf("HTTP %d", result.StatusCode)
			continue
		}
		return result, nil
	}
	return nil, lastErr
}

func (e *HTTPEngine) fetchOnce(ctx context.Context, req engine.FetchRequest) (*engine.FetchResult, error) {
	ua := UserAgentForFingerprint(e.Profile.Name)
	hdr := MergeStealthHeaders(e.Profile.Referer, ua, req.Headers)
	return rawGet(ctx, req.URL, hdr, e.Profile, 5)
}

// rawGet performs an HTTP/1.1 GET over a utls-impersonated TLS connection.
// It follows up to maxRedirects redirects.
func rawGet(ctx context.Context, rawURL string, headers map[string][]string, profile fingerprint.Profile, maxRedirects int) (*engine.FetchResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("only https is supported, got %q", u.Scheme)
	}

	spec, err := fingerprint.SpecForProfile(profile)
	if err != nil {
		return nil, err
	}

	host := u.Host
	port := u.Port()
	if port == "" {
		port = "443"
	}

	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if path == "" {
		path = "/"
	}

	// Build request with deterministic header order.
	var reqLine strings.Builder
	reqLine.WriteString("GET ")
	reqLine.WriteString(path)
	reqLine.WriteString(" HTTP/1.1\r\nHost: ")
	reqLine.WriteString(host)
	reqLine.WriteString("\r\n")

	// Sort header keys for deterministic output.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range headers[k] {
			reqLine.WriteString(k)
			reqLine.WriteString(": ")
			reqLine.WriteString(v)
			reqLine.WriteString("\r\n")
		}
	}
	reqLine.WriteString("\r\n")

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	addr := net.JoinHostPort(host, port)

	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	uconn := utls.UClient(tcpConn, &utls.Config{
		ServerName: host,
		NextProtos: []string{"http/1.1"},
	}, utls.HelloCustom)
	if err := uconn.ApplyPreset(spec); err != nil {
		tcpConn.Close()
		return nil, err
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		tcpConn.Close()
		return nil, err
	}
	defer uconn.Close()

	if _, err := io.WriteString(uconn, reqLine.String()); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(uconn)
	tp := textproto.NewReader(reader)

	statusLine, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}

	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("malformed status line: %q", statusLine)
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("malformed status code: %q", parts[1])
	}

	header, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	if statusCode >= 300 && statusCode < 400 && maxRedirects > 0 {
		loc := header.Get("Location")
		if loc == "" {
			loc = header.Get("location")
		}
		if loc != "" {
			redirectURL, err := u.Parse(loc)
			if err != nil {
				return nil, err
			}
			return rawGet(ctx, redirectURL.String(), headers, profile, maxRedirects-1)
		}
	}

	var bodyReader io.Reader = reader
	contentEncoding := header.Get("Content-Encoding")
	if contentEncoding == "" {
		contentEncoding = header.Get("content-encoding")
	}

	transferEncoding := header.Get("Transfer-Encoding")
	if transferEncoding == "" {
		transferEncoding = header.Get("transfer-encoding")
	}

	if strings.EqualFold(transferEncoding, "chunked") {
		bodyReader = newChunkedReader(reader)
	} else if cl := headerGet(header, "Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			bodyReader = io.LimitReader(reader, n)
		}
	}

	if strings.EqualFold(contentEncoding, "gzip") {
		gz, err := gzip.NewReader(bodyReader)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		bodyReader = gz
	}

	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}

	respHeaders := make(map[string]string)
	for k := range header {
		respHeaders[k] = header.Get(k)
	}

	return &engine.FetchResult{
		URL:         rawURL,
		StatusCode:  statusCode,
		Body:        body,
		Headers:     respHeaders,
		EngineUsed:  "http",
		Fingerprint: profile.Name,
	}, nil
}

func headerGet(h textproto.MIMEHeader, key string) string {
	if vs := h[key]; len(vs) > 0 {
		return vs[0]
	}
	return ""
}

type chunkedReader struct {
	r   *bufio.Reader
	buf []byte
	pos int
	eof bool
}

func newChunkedReader(r *bufio.Reader) *chunkedReader {
	return &chunkedReader{r: r}
}

func (cr *chunkedReader) Read(p []byte) (int, error) {
	if cr.eof && cr.pos >= len(cr.buf) {
		return 0, io.EOF
	}
	if cr.pos < len(cr.buf) {
		n := copy(p, cr.buf[cr.pos:])
		cr.pos += n
		return n, nil
	}
	// Read next chunk size
	line, err := cr.r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if i := strings.IndexByte(line, ';'); i >= 0 {
		line = line[:i]
	}
	size, err := strconv.ParseInt(line, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chunk size: %q", line)
	}
	if size == 0 {
		cr.eof = true
		if _, err := cr.r.ReadString('\n'); err != nil && err != io.EOF {
			return 0, err
		}
		return 0, io.EOF
	}
	cr.buf = make([]byte, size)
	if _, err := io.ReadFull(cr.r, cr.buf); err != nil {
		return 0, err
	}
	if _, err := cr.r.ReadString('\n'); err != nil && err != io.EOF {
		return 0, err
	}
	cr.pos = 0
	n := copy(p, cr.buf)
	cr.pos = n
	return n, nil
}