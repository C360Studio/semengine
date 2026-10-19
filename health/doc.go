// Package health holds the health report a service gives, and the two functions that turn
// several reports, or a component's own report, into one.
//
// # The report
//
// A [Status] names what it describes (Component), says whether it is healthy in two fields,
// carries a human-readable Message and the time it was made, and may carry the reports it was
// built from (SubStatuses) and a few numbers ([Metrics]: uptime, error count, last activity).
//
// The word in Status is one of "healthy", "degraded" or "unhealthy". Healthy is true only for
// "healthy". [NewHealthy], [NewDegraded] and [NewUnhealthy] set both fields and the timestamp, so
// build reports with them. [Status.IsDegraded], [Status.IsUnhealthy] and [Aggregate] read the word
// only, never Healthy.
//
// The JSON field names are a wire format: operators read these reports as JSON, so keep the names
// stable.
//
// A Status is a plain value, but Metrics is a pointer: copying a Status, or aggregating it, shares
// the same Metrics with the copy.
//
// # Rolling reports up
//
// [Aggregate] makes one report from several. Its word is "unhealthy" when any input's word is
// "unhealthy", otherwise "degraded" when any input's word is "degraded", otherwise "healthy".
// It looks one level deep: it reads each input's word and never looks into the input's own
// SubStatuses. An input with any other word, the empty word included, counts as healthy, and no
// inputs at all give "healthy". The result carries a copy of the inputs as its SubStatuses.
//
//	overall := health.Aggregate("platform", []health.Status{
//		health.NewHealthy("nats", "connected"),
//		health.NewDegraded("cache", "running without a warm index"),
//	})
//	// overall.Status == "degraded", overall.Healthy == false
//
// # Converting a component's report
//
// [FromComponentHealth] turns a component.HealthStatus into a Status. The word comes from Healthy
// alone: true gives "healthy" and false gives "unhealthy". The component's own Status word is not
// read, so a component that reports itself "degraded" with Healthy false is reported "unhealthy"
// (owner ruling, PR #318 comment 6271450376). Healthy is copied as is, and Metrics carries the
// component's Uptime, ErrorCount and LastCheck (as LastActivity).
//
// When the component's LastError is empty, Message is "Component healthy" or "Component
// unhealthy", following Healthy. Otherwise Message is LastError with likely secrets replaced, in
// this order:
//
//   - http, https, nats, ws and wss URLs become [URL];
//   - Unix paths (any run starting with "/") and Windows paths (C:\...) become [PATH];
//   - IPv4 addresses become [IP];
//   - ":" followed by two to five digits becomes [PORT];
//   - password, token, key, secret or credential followed by ":" or "=" and a value becomes
//     [REDACTED].
//
// The replacement works on text and can remove more than a secret: "at 12:30:45" becomes
// "at 12[PORT][PORT]", "graph/ingest" becomes "graph[PATH]", and
// "key=acme.prod.sensor.device.unit.one" becomes "[REDACTED]". The messages passed to
// NewHealthy, NewDegraded and NewUnhealthy are used as given.
//
// # What this package does not do
//
// It does not decide readiness, which is a separate question with its own endpoint (ADR-109).
// It runs no goroutine, keeps no state between calls and does no I/O.
package health
