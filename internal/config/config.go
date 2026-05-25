package config

import (
	"fmt"
	"os"
	"strings"
)

type Mode string

const (
	ModeAuto    Mode = "auto"
	ModeHTTP    Mode = "http"
	ModeBrowser Mode = "browser"
)

// MisconfigError signals a configuration error that should exit with code 2.
type MisconfigError struct {
	Msg string
}

func (e *MisconfigError) Error() string { return e.Msg }

type Config struct {
	URL      string
	Output   string
	Headers  map[string]string
	Mode     Mode
	Wait     int
	JSON     bool
	Verbose  bool
	Quiet    bool
	NoMemory bool
	Timeout  int

	CamofoxURL string

	HTTPRetries   int
	HTTPRetryWait int
}

func ApplyDefaults(cfg *Config) {
	if cfg.CamofoxURL == "" {
		cfg.CamofoxURL = os.Getenv("GRAB_CAMOFOX_URL")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	if cfg.HTTPRetries <= 0 {
		cfg.HTTPRetries = 3
	}
	if cfg.HTTPRetryWait <= 0 {
		cfg.HTTPRetryWait = 2
	}
}

func Validate(cfg *Config) error {
	if cfg.URL == "" {
		return fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(cfg.URL, "http://") && !strings.HasPrefix(cfg.URL, "https://") {
		return fmt.Errorf("url must start with http:// or https://")
	}
	switch cfg.Mode {
	case ModeAuto, ModeHTTP, ModeBrowser:
	default:
		return &MisconfigError{Msg: fmt.Sprintf("invalid mode %q: must be auto, http, or browser", cfg.Mode)}
	}
	if cfg.Verbose && cfg.Quiet {
		return fmt.Errorf("-v and -q are mutually exclusive")
	}
	if cfg.Mode == ModeBrowser && cfg.CamofoxURL == "" {
			return &MisconfigError{Msg: "--mode browser requires GRAB_CAMOFOX_URL to be set"}
		}
	return nil
}