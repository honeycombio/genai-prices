package genaiprices

// Name identifies this library in telemetry that records cost-estimate
// provenance.
const Name = "genai-prices"

// DataSource identifies the upstream project the embedded price catalog
// (data.json) is synced from. The exact upstream data version last synced is
// tracked in upstream-watch/requirements.txt. See SYNCING.md.
const DataSource = "pydantic/genai-prices"

// Version is the honeycombio/genai-prices release version. It tracks THIS
// library's releases, which are deliberately not 1:1 with upstream
// pydantic/genai-prices data syncs: we can ship engine changes without a data
// bump, or sync data without a code release. Bump it when cutting a release.
const Version = "0.0.1"
