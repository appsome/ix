package pubsub

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counters every broker keeps, labelled by topic. Topics are the project's
// own small set of strings, so the label stays bounded.
var (
	published = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pubsub_published_total",
		Help: "Payloads a process published, by topic.",
	}, []string{"topic"})
	received = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pubsub_received_total",
		Help: "Payloads that arrived from Redis or Postgres for this process's subscribers, by topic.",
	}, []string{"topic"})
	dropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "pubsub_dropped_total",
		Help: "Payloads a subscriber missed because its channel was full, by topic.",
	}, []string{"topic"})
	connected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pubsub_connected",
		Help: "1 while the Redis broker reaches Redis, 0 while it does not; payloads published meanwhile are lost.",
	})
)
