// Package stowage computes how ordered vessel groups fit a chamber's
// effective length before convoy plans are created.
package stowage

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// DefaultConfig applies to chambers without an explicit configuration.
var DefaultConfig = Config{EffectiveLengthM: 120, SafetySpacingM: 5}

// Config bounds one lockage: vessel lengths plus safety spacing between
// neighbours must stay within the chamber's effective length.
type Config struct {
	EffectiveLengthM float64   `json:"effective_length_m"`
	SafetySpacingM   float64   `json:"safety_spacing_m"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (c Config) validate() error {
	if c.EffectiveLengthM <= 0 || c.EffectiveLengthM > 10000 {
		return fmt.Errorf("effective length must be within (0, 10000] metres, got %v", c.EffectiveLengthM)
	}
	if c.SafetySpacingM < 0 || c.SafetySpacingM >= c.EffectiveLengthM {
		return fmt.Errorf("safety spacing must be within [0, %v) metres, got %v", c.EffectiveLengthM, c.SafetySpacingM)
	}
	return nil
}

func (c Config) stamped(now time.Time) Config {
	c.UpdatedAt = now.UTC()
	return c
}

// Registry keeps one configuration per chamber, falling back to defaults.
type Registry struct {
	mu       sync.RWMutex
	defaults Config
	configs  map[string]Config
	now      func() time.Time
}

func NewRegistry(defaults Config, now func() time.Time) (*Registry, error) {
	if now == nil {
		now = time.Now
	}
	if err := defaults.validate(); err != nil {
		return nil, fmt.Errorf("invalid default stowage config: %w", err)
	}
	return &Registry{
		defaults: defaults.stamped(now()), configs: make(map[string]Config), now: now,
	}, nil
}

// Get returns the chamber's configuration, or the default when unset. The
// second result reports whether the value was explicitly configured.
func (r *Registry) Get(chamberID string) (Config, bool) {
	r.mu.RLock()
	config, ok := r.configs[chamberID]
	r.mu.RUnlock()
	if ok {
		return config, true
	}
	return r.defaults, false
}

// Set validates and stores a chamber-specific configuration.
func (r *Registry) Set(chamberID string, config Config) (Config, error) {
	if chamberID == "" {
		return Config{}, errors.New("chamber id is required")
	}
	if err := config.validate(); err != nil {
		return Config{}, err
	}
	config = config.stamped(r.now())
	r.mu.Lock()
	r.configs[chamberID] = config
	r.mu.Unlock()
	return config, nil
}
