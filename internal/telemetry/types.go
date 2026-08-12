package telemetry

import (
	"context"
	"errors"
	"time"
)

var ErrMissingTimestamp = errors.New("telemetry record has no event timestamp")

type Attr struct{ Key, Value string }
type Device struct{ ID, Name, MAC, Model, Site, IP string }

type Emitter interface {
	Gauge(context.Context, string, float64, ...Attr) error
	Counter(context.Context, string, float64, ...Attr) error
	LogEvent(context.Context, string, string, time.Time, ...Attr) error
	StartSpan(context.Context, string, ...Attr) (context.Context, func(error))
	WithTenant(string) Emitter
	WithDevice(Device) Emitter
}
