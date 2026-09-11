# homelab-go

Observability and serving packages for small Go HTTP services.

| | |
|---|---|
| `logging` | installs slog's default handler: JSON on stdout, with the active span's trace ids |
| `tracing` | OTLP/gRPC span export, a server-span middleware, and a span wrapper for store methods |
| `profiling` | pprof endpoints on a dedicated listener, separate from the app's own mux |
| `metrics` | an HTTP latency histogram and in-flight gauge, labelled by matched route |
| `serve` | `http.Server` with timeouts, and shutdown on SIGINT/SIGTERM |
| `etag` | strong ETags over embedded assets, served so a repeat request costs a 304 |
| `render` | executes a template into a buffer, so a response is the whole page or a clean 500 |
| `access` | reads the identity header Cloudflare Access sets |

```sh
go get github.com/jchevertonwynne/homelab-go
```

## metrics

Exposes `http_request_duration_seconds` (histogram, `prometheus.DefBuckets`,
labelled `method`/`route`/`status`) and `http_requests_in_flight` (gauge) on
the default registry, served by `Handler`.

`Instrument` labels by the matched `http.ServeMux` pattern rather than the raw
path, so cardinality is bounded by the number of routes. Anything no router
matched is labelled `unmatched`.

**The series names, label names and label order are a stable contract.** A
dashboard or alert built against them keeps working; renaming one yields an
empty panel rather than an error. `status` is a string label holding the
numeric code — it is not named `code`.

`WithRouters` adds routers consulted after the handler itself, for a layered
mux whose outer patterns are less specific than its inner ones.
`WithExcludedRoutes` keeps named routes out of both the histogram and the
gauge, for long-lived streams that would otherwise make both read wrong for as
long as the connection lasts.

## tracing

`Init` configures the global `TracerProvider` to batch spans over OTLP/gRPC.
An empty endpoint is a no-op, leaving the no-op tracer in place. `Middleware`
wraps a handler in a server span.

`Op` and `Do` run a function inside a child span and record its error on it.
`Do` is for functions returning only an error; `Op` is generic over a return
value. The tracer is a parameter rather than package state, because
OpenTelemetry names a tracer after the package being instrumented.

## etag

`Of` returns a strong validator over content. `Map` walks an `fs.FS` and
returns URL path to ETag. `Handler` serves that FS with the ETag and
`Cache-Control: no-cache`, which `http.FileServerFS` cannot do for an
`embed.FS` on its own. `ServeBytes` covers the single-asset case.

`ServeBytes` does not set `Content-Type`; set it before calling.
`http.ServeContent` infers it from the name's extension via
`mime.TypeByExtension`, which reads the system mime database and is not
dependable for every type.

## render

`HTML` calls `Execute`, `Named` calls `ExecuteTemplate`. Both render into a
buffer first, respond 500 on failure having written nothing partial, and
return the error for the caller to log.

## access

`EmailHeader` is the header Cloudflare Access sets. `NormalizeEmail` collapses
case and surrounding whitespace. `Email` returns the normalised address,
falling back to a supplied development user only when the header is absent,
never in preference to it.

There is no middleware here and nothing returns a 403. `Email` reports
`ok == false` when it has no identity and says nothing about what that should
mean; the status code, the log line and the decision belong to the caller.
