# Raw-first Telemetry Implementation Plan

1. Change Middleware decoder output and tests to preserve Raw values.
2. Stop Platform Raw ingestion from creating calculated telemetry.
3. Read Latest and History from Raw plus current Register Metadata.
4. Run both Go test suites and the release build.
