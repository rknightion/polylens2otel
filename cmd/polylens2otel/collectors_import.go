package main

import (
	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/collectors/lens"
	"github.com/rknightion/polylens2otel/internal/collectors/lenscdr"
	"github.com/rknightion/polylens2otel/internal/collectors/phone"
	"github.com/rknightion/polylens2otel/internal/collectors/selfobs"
)

func registerAll(d collector.Deps) {
	lens.Register(d)
	lenscdr.Register(d)
	phone.Register(d)
	selfobs.Register(d)
}
