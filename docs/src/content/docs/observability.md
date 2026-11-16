---
title: Observability
description: Traces, metrics and logs with OpenTelemetry, locally and in production.
---

The [observability](/esquadro/blueprints/reference/observability/) blueprint
makes a project send its **traces**, **metrics** and **logs** with
[OpenTelemetry](https://opentelemetry.io), over OTLP, to any backend. The
`api` and `worker` starters have it; `esquadro add observability` adds it to
any project.

## What it records

- **Each request** (with `http`) is a span named after its route, such as
  `GET /docs` or `POST /api/v1/`, with the API's operation inside it
  (`CreateCategory`), and the standard HTTP metrics: duration, sizes,
  status. A caller's `traceparent` header continues its trace.
- **Each query** (with `postgres`) is a span, with its SQL, and the pool's
  metrics: connections in use, idle, waited for.
- **Each job** (with `jobs`) is a span, in the trace of whoever enqueued it:
  the request that enqueued an email and the job that sent it are one
  trace.
- **Every log** goes to OpenTelemetry too, and the lines on stdout carry the
  `trace_id` and `span_id` of their request, to go from a log line to its
  trace:

```
level=INFO msg=request method=POST route=/api/v1/ status=201 ... trace_id=5cfcb929... span_id=93295998...
```

- **The Go runtime:** memory, goroutines, garbage collection.

## Locally

With `compose`, `task up` also starts
[grafana/otel-lgtm](https://github.com/grafana/docker-otel-lgtm): Grafana at
http://localhost:3000 (user and password `admin`), with Tempo for traces,
Loki for logs and Prometheus for metrics. Outside production, the app sends
there without any configuration: run it, make a request, and open
**Explore** in Grafana.

Ports that are taken elsewhere change in the compose block:

```esquadro
blueprint compose {
  ports ["grafana:3300", "otlp:4319"]
}
```

## In production

Nothing is sent until `OTEL_EXPORTER_OTLP_ENDPOINT` is set; without it, the
telemetry costs nothing. The standard OpenTelemetry variables apply:

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp-gateway-prod-us-east-0.grafana.net/otlp
OTEL_EXPORTER_OTLP_HEADERS="Authorization=Basic <token>"
```

That is all it takes for Grafana Cloud, Honeycomb, Datadog, New Relic, or a
collector of your own: each documents its endpoint and header.

## Sampling

Every trace is kept by default. To keep a share of them:

```esquadro
blueprint observability {
  sample_ratio 0.1
}
```

`SHOP_TRACE_SAMPLE_RATIO=0.1` changes it without regenerating, and
`OTEL_TRACES_SAMPLER` replaces the sampler altogether. A trace started by a
caller keeps the caller's decision.
