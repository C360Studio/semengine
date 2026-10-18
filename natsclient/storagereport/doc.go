// Package storagereport produces and reads the account storage report.
//
// A StorageInventoryCollector lists every JetStream-backed resource the account holds (ordinary
// streams, KV buckets and ObjectStores, whoever created them) through AccountStreamLister. A
// Publisher derives each resource's growth, projection and pressure from successive inventories
// and writes one KV key per resource, plus the account row, to the storage report bucket. A
// Consumer watches that bucket and serves the current report as a Snapshot, telling an optional
// Observer about each change.
//
// The bucket itself (its name, owner and retention) is declared in graph/kvcatalog; this package
// holds no bucket name and imports no graph package. It reads the account through natsclient.
package storagereport
