package cli

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

// reorderArgs moves flags with their values before positional arguments.
// Lets users write "grab URL -o file" like curl, instead of requiring flags first.
func reorderArgs(args []string) []string {
	// Flag names that consume a following value argument.
	hasValue := map[string]bool{
		"o": true, "H": true, "A": true, "mode": true,
		"timeout": true, "wait": true,
	}
	flags := make([]string, 0)
	positional := make([]string, 0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && !hasValue[flagName(a)] {
			// Boolean flag (e.g. -v, -s, -json): no value follows.
			flags = append(flags, a)
		} else if strings.HasPrefix(a, "-") && strings.Contains(a, "=") {
			// --mode=auto, -timeout=30: value inline, single arg.
			flags = append(flags, a)
		} else if strings.HasPrefix(a, "-") {
			// Flag with following value: -o file, -H "K: V"
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

// flagName extracts the bare flag name from -x, --xxx, or --xxx=yyy.
func flagName(arg string) string {
	s := strings.TrimLeft(arg, "-")
	if i := strings.IndexByte(s, '='); i >= 0 {
		s = s[:i]
	}
	return s
}

func Run(args []string) error {
		// Reorder: move flags before positional args so -o after URL works like curl.
		args = reorderArgs(args)
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