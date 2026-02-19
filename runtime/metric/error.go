// Package metric provides a small Prometheus error-counter and an HTTP service
// that exposes /metrics.
//
// The counter name is generic (xi_errors_total) rather than domain-specific;
// projects that need a different name can register their own CounterVec and
// call RecordErrorTo.
package metric

import (
	"log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// errorsTotal counts recorded errors by stable code. Registered once at
// package init via promauto.
var errorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "xi_errors_total",
	Help: "Total number of recorded errors, labelled by error code.",
}, []string{"code"})

// RecordError increments the error counter for code and logs message + err.
// It is a no-op when err is nil, so call sites can wrap fallible operations
// without a surrounding nil check.
func RecordError(code, message string, err error) {
	RecordErrorTo(errorsTotal, code, message, err)
}

// RecordErrorTo is RecordError against a caller-supplied counter, for projects
// that register their own metric name/labels.
func RecordErrorTo(counter *prometheus.CounterVec, code, message string, err error) {
	if err == nil {
		return
	}
	counter.WithLabelValues(code).Inc()
	log.Printf("%s: %s", code, message)
	log.Printf("%s: %v", code, err)
}
