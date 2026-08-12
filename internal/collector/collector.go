package collector

import (
	"context"
	"time"

	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

type Collector interface {
	ID() string
	Interval() time.Duration
	Collect(context.Context, telemetry.Emitter) error
}
type Registry struct{ entries []Collector }

func NewRegistry() *Registry { return &Registry{} }
func (r *Registry) Register(c Collector) {
	if c == nil {
		return
	}
	r.entries = append(r.entries, c)
}
func (r *Registry) Entries() []Collector { return append([]Collector(nil), r.entries...) }

type Deps struct {
	Config   *config.Config
	Emitter  telemetry.Emitter
	Registry *Registry
	Services map[string]any
}

func (d Deps) Service(name string) any {
	if d.Services == nil {
		return nil
	}
	return d.Services[name]
}
