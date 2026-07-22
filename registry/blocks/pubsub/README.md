# Block: `pubsub`

In-proc, Postgres LISTEN/NOTIFY or Redis broker backing GraphQL subscriptions.
The broker logic is imported from `runtime/pubsub`; this block vendors the project
glue.

Status: **available**. `ix add pubsub` vendors:

- `internal/pubsub/wire.go` — `New(ctx, Options{RedisURL, RedisPrefix, DSN,
  Channels})`, which builds a Redis broker when `RedisURL` is set, a Postgres
  LISTEN/NOTIFY broker when `DSN` is set and an in-process one otherwise;
  `NewBroker(ctx, dsn, channels...)`, the 0.1 signature; and a re-export of the
  runtime `Broker` interface.

## Choosing a broker

| Broker | Reaches other processes | Needs | Delivery |
|---|---|---|---|
| in-process | no | nothing | in-process only |
| Postgres | yes, on the listed channels | the database; payloads up to 8000 bytes | at most once |
| Redis | yes, every topic under the prefix | a Redis | at most once |

A Redis or Postgres broker that cannot connect fails `New`. Every broker
counts `pubsub_published_total`, `pubsub_received_total` and
`pubsub_dropped_total` by topic; the Redis broker also sets `pubsub_connected`.
Publish ids rather than whole objects and read the state on receipt: a payload
published while a listener is away is lost to it.

It patches `cmd/api/main.go` at the `ix:wire-services` anchor to construct the
broker and close it on shutdown.

## Wiring it in (manual next steps)

`resolver.New` takes only the database handle, so thread the broker in yourself:

1. Add a `Broker` field to the resolver root and pass `broker` through
   `resolver.New`.
2. Back each Subscription resolver with `broker.Subscribe(ctx, topic)` and
   publish with `broker.Publish(ctx, topic, payload)`.
3. For cross-process delivery, set `Options.RedisURL` (and `RedisPrefix` when
   deployments share a Redis), or set `Options.Channels` to the topics you
   publish on so a Postgres broker LISTENs on them.
