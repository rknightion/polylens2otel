package main

import (
	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/collectors/lens"
	"github.com/rknightion/polylens2otel/internal/collectors/lenscdr"
	"github.com/rknightion/polylens2otel/internal/collectors/phone"
	"github.com/rknightion/polylens2otel/internal/collectors/selfobs"
)

func registerAll(d collector.Deps) { //nolint:unused // Frozen L0 seam; L14 wires it into main.
	lens.Register(d)
	lenscdr.Register(d)
	phone.Register(d)
	selfobs.Register(d)
}
