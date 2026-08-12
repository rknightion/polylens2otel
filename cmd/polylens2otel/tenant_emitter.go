package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const discoveryTenantID = "_discovery"

// tenantEmitter keeps process-level telemetry inside the same tenant boundary
// as device telemetry. Explicit WithTenant calls select one tenant; otherwise
// an event is fanned out across the configured or discovered tenants.
type tenantEmitter struct {
	base  telemetry.Emitter
	scope *tenantScope
}

type tenantScope struct {
	mu      sync.RWMutex
	tenants []string
}

func newTenantEmitter(base telemetry.Emitter, tenants []string) *tenantEmitter {
	emitter := &tenantEmitter{base: base, scope: &tenantScope{}}
	emitter.SetTenants(tenants)
	return emitter
}

func (e *tenantEmitter) SetTenants(tenants []string) {
	seen := make(map[string]struct{}, len(tenants))
	clean := make([]string, 0, len(tenants))
	for _, tenant := range tenants {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" {
			continue
		}
		if _, ok := seen[tenant]; ok {
			continue
		}
		seen[tenant] = struct{}{}
		clean = append(clean, tenant)
	}
	e.scope.mu.Lock()
	e.scope.tenants = clean
	e.scope.mu.Unlock()
}

func (e *tenantEmitter) scoped() []telemetry.Emitter {
	e.scope.mu.RLock()
	tenants := append([]string(nil), e.scope.tenants...)
	e.scope.mu.RUnlock()
	if len(tenants) == 0 {
		tenants = []string{discoveryTenantID}
	}
	out := make([]telemetry.Emitter, 0, len(tenants))
	for _, tenant := range tenants {
		out = append(out, e.base.WithTenant(tenant))
	}
	return out
}

func (e *tenantEmitter) Gauge(ctx context.Context, name string, value float64, attrs ...telemetry.Attr) error {
	var errs []error
	for _, emitter := range e.scoped() {
		errs = append(errs, emitter.Gauge(ctx, name, value, attrs...))
	}
	return errors.Join(errs...)
}

func (e *tenantEmitter) Counter(ctx context.Context, name string, value float64, attrs ...telemetry.Attr) error {
	var errs []error
	for _, emitter := range e.scoped() {
		errs = append(errs, emitter.Counter(ctx, name, value, attrs...))
	}
	return errors.Join(errs...)
}

func (e *tenantEmitter) LogEvent(ctx context.Context, event, body string, ts time.Time, attrs ...telemetry.Attr) error {
	var errs []error
	for _, emitter := range e.scoped() {
		errs = append(errs, emitter.LogEvent(ctx, event, body, ts, attrs...))
	}
	return errors.Join(errs...)
}

func (e *tenantEmitter) StartSpan(ctx context.Context, name string, attrs ...telemetry.Attr) (context.Context, func(error)) {
	spanCtx := ctx
	ends := make([]func(error), 0)
	for i, emitter := range e.scoped() {
		childCtx, end := emitter.StartSpan(ctx, name, attrs...)
		if i == 0 {
			spanCtx = childCtx
		}
		ends = append(ends, end)
	}
	return spanCtx, func(err error) {
		for _, end := range ends {
			end(err)
		}
	}
}

func (e *tenantEmitter) WithTenant(id string) telemetry.Emitter {
	if strings.TrimSpace(id) == "" {
		return e
	}
	return e.base.WithTenant(id)
}

func (e *tenantEmitter) WithDevice(device telemetry.Device) telemetry.Emitter {
	return &tenantEmitter{base: e.base.WithDevice(device), scope: e.scope}
}
