# homelab-go

The observability and serving glue shared by the apps on the k3s cluster in
[jchevertonwynne/homelab](https://github.com/jchevertonwynne/homelab).

| | |
|---|---|
| `logging` | slog's default handler: JSON on stdout, with the active span's trace ids |
| `tracing` | OTLP/gRPC spans to Alloy, and the server-span middleware |
| `profiling` | pprof endpoints on a listener that is never routed |
| `metrics` | the frozen Prometheus contract, and the route-label decision |
| `serve` | `http.Server` timeouts and SIGTERM shutdown |

These were seven byte-identical copies before this module existed, one per
app, and `metrics` had already forked into three variants — one of them
untested. That is the problem this solves.

## The metric contract is frozen

`homelab` builds one dashboard panel per app, plus the `AppErrorBurst` alert,
from these exact series names, label names and label order. A rename produces
an empty panel rather than an error, so nothing in `metrics` is safe to tidy:
`status` stays a string label holding the numeric code (it is not `code`), the
labels stay in `method`/`route`/`status` order, and the buckets stay
`prometheus.DefBuckets`.

## Consuming it

Public on purpose. It was private to begin with, and every app then had to
commit a `vendor/` tree so its image build needed no credential — 407 MB of
third-party code across seven repos, because vendoring is all-or-nothing and
one of those apps pulls in pure-Go SQLite. Nothing in here is worth that:
a slog handler, an OTLP exporter setup, pprof wiring and one histogram.

```sh
go get -u github.com/jchevertonwynne/homelab-go && go mod tidy
```
