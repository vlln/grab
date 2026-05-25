package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vlln/grab/internal/config"
	"github.com/vlln/grab/internal/engine"
	"github.com/vlln/grab/internal/engine/browser"
	httpengine "github.com/vlln/grab/internal/engine/http"
	"github.com/vlln/grab/internal/fingerprint"
	"github.com/vlln/grab/internal/memory"
)

type Orchestrator struct {
	Config   *config.Config
	Memory   *memory.Store
	Logger   *slog.Logger
}

func New(cfg *config.Config, mem *memory.Store) *Orchestrator {
	level := slog.LevelInfo
	if cfg.Quiet {
		level = slog.LevelError
	}
	return &Orchestrator{
		Config: cfg,
		Memory: mem,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
	}
}

func (o *Orchestrator) Run() error {
	domain := memory.Domain(o.Config.URL)

	req := engine.FetchRequest{
		URL:     o.Config.URL,
		Headers: o.Config.Headers,
		Timeout: time.Duration(o.Config.Timeout) * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sig)
	}()
	var lastErr error

	// Browser-only mode: skip HTTP engines entirely.
	if o.Config.Mode == config.ModeBrowser {
		return o.runBrowserOnly(ctx, req)
	}

	// [1] Memory hit: try cached fingerprint first, but only for auto/http modes.
	if !o.Config.NoMemory {
		if entry, ok := o.Memory.Get(domain); ok {
			o.log("[1/3] memory hit", "fingerprint", entry.Fingerprint)
			for _, p := range fingerprint.DefaultRotation {
				if p.Name == entry.Fingerprint {
					result, err := o.tryHTTP(ctx, req, p, true)
					if err == nil {
						o.Memory.Set(domain, p.Name, entry.Referer)
						o.Memory.Save()
						return o.output(result)
					}
					lastErr = err
					break
				}
			}
		}
	}

	// [2] HTTP rotation
	profiles := fingerprint.Rotation("")
	for i, p := range profiles {
		o.log(fmt.Sprintf("[2/3] http rotation (%d/%d)", i+1, len(profiles)),
			"fingerprint", p.Name)
		result, err := o.tryHTTP(ctx, req, p, false)
		if err == nil {
			if !o.Config.NoMemory {
				o.Memory.Set(domain, p.Name, p.Referer)
				o.Memory.Save()
			}
			return o.output(result)
		}
		lastErr = err
	}

	if o.Config.Mode == config.ModeHTTP {
		return fmt.Errorf("all http engines failed: %w", lastErr)
	}

	// [3] Browser fallback
	if o.Config.CamofoxURL == "" {
		return fmt.Errorf("all http engines failed and no camofox configured: %w", lastErr)
	}

	o.log("[3/3] browser fallback", "url", o.Config.CamofoxURL)
	result, err := o.tryBrowser(ctx, req)
	if err != nil {
		return fmt.Errorf("browser fallback failed: %w", err)
	}
	if !o.Config.NoMemory {
		o.Memory.Set(domain, "browser", "")
		o.Memory.Save()
	}
	return o.output(result)
}

func (o *Orchestrator) runBrowserOnly(ctx context.Context, req engine.FetchRequest) error {
	o.log("[1/1] browser only", "url", o.Config.CamofoxURL)
	result, err := o.tryBrowser(ctx, req)
	if err != nil {
		return fmt.Errorf("browser failed: %w", err)
	}
	return o.output(result)
}

func (o *Orchestrator) tryHTTP(ctx context.Context, req engine.FetchRequest, p fingerprint.Profile, fromMemory bool) (*engine.FetchResult, error) {
	eng := httpengine.NewHTTPEngine(p, o.Config.HTTPRetries, time.Duration(o.Config.HTTPRetryWait)*time.Second)
	result, err := eng.Fetch(ctx, req)
	if err != nil {
		o.log("  failed", "fingerprint", p.Name, "error", err)
		return nil, err
	}
	result.FromMemory = fromMemory
	o.log("  success", "fingerprint", p.Name, "status", result.StatusCode, "from_memory", fromMemory)
	return result, nil
}

func (o *Orchestrator) tryBrowser(ctx context.Context, req engine.FetchRequest) (*engine.FetchResult, error) {
	engine := browser.NewBrowserEngine(o.Config.CamofoxURL)
	// Add wait time to the timeout if specified
	if o.Config.Wait > 0 {
		req.Timeout += time.Duration(o.Config.Wait) * time.Second
	}
	result, err := engine.Fetch(ctx, req)
	if err != nil {
		o.log("  failed", "engine", "browser", "error", err)
		return nil, err
	}
	o.log("  success", "engine", "browser")
	return result, nil
}

func (o *Orchestrator) output(result *engine.FetchResult) error {
	if o.Config.JSON {
		return o.outputJSON(result)
	}
	return o.outputRaw(result)
}

func (o *Orchestrator) outputRaw(result *engine.FetchResult) error {
	var w io.Writer = os.Stdout
	var f *os.File
	if o.Config.Output != "" {
		var err error
		f, err = os.Create(o.Config.Output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	_, err := w.Write(result.Body)
	return err
}

type jsonOutput struct {
	URL         string            `json:"url"`
	StatusCode  int               `json:"status_code"`
	EngineUsed  string            `json:"engine_used"`
	Fingerprint string            `json:"fingerprint"`
	FromMemory  bool              `json:"from_memory"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
}

func (o *Orchestrator) outputJSON(result *engine.FetchResult) error {
	out := jsonOutput{
		URL:         result.URL,
		StatusCode:  result.StatusCode,
		EngineUsed:  result.EngineUsed,
		Fingerprint: result.Fingerprint,
		FromMemory:  result.FromMemory,
		Headers:     result.Headers,
		Body:        string(result.Body),
	}

	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func (o *Orchestrator) log(msg string, args ...any) {
	if o.Config.Quiet {
		return
	}
	if !o.Config.Verbose && !strings.HasPrefix(msg, "[") {
		return
	}
	if !o.Config.Verbose && strings.HasPrefix(msg, "  ") {
		return
	}
	all := append([]any{"msg", msg}, args...)
	o.Logger.Info("[grab]", all...)
}