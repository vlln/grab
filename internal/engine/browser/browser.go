package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/vlln/grab/internal/engine"
)

const defaultTimeout = 120 * time.Second

type BrowserEngine struct {
	BaseURL string
	UserId  string
	client  *http.Client
}

func NewBrowserEngine(baseURL string) *BrowserEngine {
	host, _ := os.Hostname()
	return &BrowserEngine{
		BaseURL: baseURL,
		UserId:  "grab-" + host,
		client:  &http.Client{Timeout: defaultTimeout},
	}
}

func (e *BrowserEngine) Name() string { return "browser" }

func (e *BrowserEngine) Fetch(ctx context.Context, req engine.FetchRequest) (*engine.FetchResult, error) {
	tabId, err := e.openTab(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("browser open: %w", err)
	}
	defer e.closeTab(tabId)

	if req.Timeout > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(req.Timeout):
		}
	}

	html, err := e.getHTML(ctx, tabId)
	if err != nil {
		return nil, fmt.Errorf("browser get html: %w", err)
	}

	return &engine.FetchResult{
		URL:        req.URL,
		StatusCode: 200,
		Body:       []byte(html),
		EngineUsed: "browser",
	}, nil
}

type tabOpenResp struct {
	TabId string `json:"tabId"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

type evaluateResp struct {
	OK     bool   `json:"ok"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

func (e *BrowserEngine) openTab(ctx context.Context, req engine.FetchRequest) (string, error) {
	body := map[string]string{
		"url":    req.URL,
		"userId": e.UserId,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.BaseURL+"/tabs/open", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Connection", "keep-alive")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	var r tabOpenResp
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", fmt.Errorf("parse error: %s", string(respBody))
	}
	if r.Error != "" {
		return "", fmt.Errorf("%s", r.Error)
	}
	if r.TabId == "" {
		return "", fmt.Errorf("no tabId in response: %s", string(respBody))
	}
	return r.TabId, nil
}

func (e *BrowserEngine) getHTML(ctx context.Context, tabId string) (string, error) {
	body := map[string]string{
		"userId":     e.UserId,
		"expression": "document.documentElement.outerHTML",
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.BaseURL+"/tabs/"+tabId+"/evaluate", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	var r evaluateResp
	if err := json.Unmarshal(respBody, &r); err != nil {
		return "", fmt.Errorf("parse error: %s", string(respBody))
	}
	if r.Error != "" {
		return "", fmt.Errorf("%s", r.Error)
	}
	return r.Result, nil
}

func (e *BrowserEngine) closeTab(tabId string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "DELETE", e.BaseURL+"/tabs/"+tabId, nil)
	if err != nil {
		return
	}
	resp, err := e.client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}