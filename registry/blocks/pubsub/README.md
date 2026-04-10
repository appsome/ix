# Block: `pubsub`

In-proc + Postgres LISTEN/NOTIFY broker backing GraphQL subscriptions. The
broker logic is imported from `runtime/pubsub`; this block vendors the project
glue.

Status: **available**. `ix add pubsub` vendors:

- `internal/pubsub/wire.go` — `NewBroker(ctx, dsn, channels...)` (a Postgres
  LISTEN/NOTIFY broker when `dsn` is set, in-process otherwise) and a re-export
  of the runtime `Broker` interface.

It patches `cmd/api/main.go` at the `ix:wire-services` anchor to construct the
broker and close it on shutdown.

## Wiring it in (manual next steps)

`resolver.New` takes only the database handle, so thread the broker in yourself:

1. Add a `Broker` field to the resolver root and pass `broker` through
   `resolver.New`.
2. Back each Subscription resolver with `broker.Subscribe(ctx, topic)` and
   publish with `broker.Publish(ctx, topic, payload)`.
3. Pass the topics you publish on as extra args to `pubsub.NewBroker` so a
   Postgres broker LISTENs on the matching channels for cross-process delivery.
