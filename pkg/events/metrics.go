package events

import "github.com/prometheus/client_golang/prometheus"

// PublishTotal counts domain-event publish attempts, labeled by event type and
// result ("success" | "error"). Because publishing is best-effort (failures are
// swallowed rather than returned to the HTTP client), this counter is the
// primary signal that events are failing to reach the broker.
//
// It is defined here but registered on the application's scrape registry by
// middleware.NewMetrics, so it appears on the /metrics endpoint. It is safe to
// increment even when metrics are disabled (the counter simply is not exposed).
var PublishTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "domain_events_published_total",
		Help: "Domain event publish attempts, labeled by event type and result (success|error).",
	},
	[]string{"type", "result"},
)
