# Block: `metrics`

Prometheus metrics endpoint + coded error registry. The /metrics service and
the error-counter logic are imported from `runtime/metric`; this block vendors
the project glue.

Status: **available**. `ix add metrics` vendors:

- `internal/metric/wire.go` — `Start(ctx)` (launches the /metrics server on
  `METRIC_PORT`, drained on shutdown), `RecordError(code, message, err)`, and a
  re-export of the runtime `Service` type.

It patches `cmd/api/main.go` at the `ix:wire-services` anchor to start the
metrics service alongside the API server.

## Use

- Set `METRIC_PORT` in the environment; scrape `http://<host>:<METRIC_PORT>/metrics`.
- Call `metric.RecordError("CODE001", "what failed", err)` at error sites; it is
  a no-op when `err` is nil and increments a Prometheus counter labelled by code.
