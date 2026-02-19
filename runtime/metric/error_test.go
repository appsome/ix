package metric

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordErrorTo(t *testing.T) {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_errors_total",
		Help: "test",
	}, []string{"code"})

	// nil error is a no-op.
	RecordErrorTo(c, "E1", "should not count", nil)
	if got := testutil.ToFloat64(c.WithLabelValues("E1")); got != 0 {
		t.Fatalf("nil error counted: got %v, want 0", got)
	}

	RecordErrorTo(c, "E1", "boom", errors.New("boom"))
	RecordErrorTo(c, "E1", "boom again", errors.New("boom"))
	if got := testutil.ToFloat64(c.WithLabelValues("E1")); got != 2 {
		t.Fatalf("got %v, want 2", got)
	}
}
