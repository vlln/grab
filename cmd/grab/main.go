package grab

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vlln/grab/internal/config"
	"github.com/vlln/grab/internal/memory"
	"github.com/vlln/grab/internal/orchestrator"
)

type headersFlag map[string]string

func (h headersFlag) String() string { return "" }
func (h headersFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, ": ")
	if !ok {
		if k, v, ok = strings.Cut(s, ":"); !ok {
			return fmt.Errorf("invalid header format: %q (expected Key: Value)", s)
		}
	}
	if _, exists := h[k]; exists {
		return fmt.Errorf("duplicate header: %q", k)
	}
	h[k] = strings.TrimSpace(v)
	return nil
}
func (h headersFlag) IsBoolFlag() bool { return false }

func Run(args []string) error {
	fs := flag.NewFlagSet("grab", flag.ContinueOnError)
	fs.SetOutput(nil)

	var cfg config.Config
	var headers headersFlag = make(map[string]string)
	var mode string

	fs.StringVar(&cfg.Output, "o", "", "output file path")
	fs.Var(&headers, "H", "extra header (Key: Value), repeatable")
	fs.StringVar(&mode, "mode", "auto", "engine mode: auto, http, or browser")
	fs.IntVar(&cfg.Wait, "wait", 0, "seconds to wait after page load (browser mode)")
	fs.BoolVar(&cfg.JSON, "json", false, "output JSON envelope")
	fs.BoolVar(&cfg.Verbose, "v", false, "verbose logging to stderr")
	fs.StringVar(&cfg.UserAgent, "A", "", "custom User-Agent header")
	fs.BoolVar(&cfg.Silent, "s", false, "silent mode")
	fs.BoolVar(&cfg.NoMemory, "no-memory", false, "skip domain memory")
	fs.IntVar(&cfg.Timeout, "timeout", 30, "request timeout in seconds")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cfg.Mode = config.Mode(mode)
	cfg.URL = fs.Arg(0)

	if cfg.UserAgent != "" {
		if headers == nil {
			headers = make(map[string]string)
		}
		headers["User-Agent"] = cfg.UserAgent
	}
	cfg.Headers = headers

	config.ApplyDefaults(&cfg)
	if err := config.Validate(&cfg); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	memPath := filepath.Join(home, ".grab", "memory.json")
	mem := memory.NewStore(memPath)
	_ = mem.Load()

	orch := orchestrator.New(&cfg, mem)
	return orch.Run()
}