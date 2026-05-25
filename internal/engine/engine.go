package engine

import (
	"context"
	"time"
)

type FetchRequest struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

type FetchResult struct {
	URL         string
	StatusCode  int
	Body        []byte
	Headers     map[string]string
	EngineUsed  string
	Fingerprint string
	FromMemory  bool
	Error       error
}

type Engine interface {
	Name() string
	Fetch(ctx context.Context, req FetchRequest) (*FetchResult, error)
}