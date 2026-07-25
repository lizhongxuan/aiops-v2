# Workflow edit v2 contract

This package is the single editable source for the transport-neutral Workflow edit v2 domain contract.

- Go structs and JSON tags in this package are authoritative.
- `internal/integrations/workfloweditor/schema_v2.go` reflects these types and adds the discriminated operation union required by JSON Schema.
- TypeScript consumers are generated from that schema; they must not independently redefine fields.
- Adapters may translate legacy HTTP/MCP shapes, but they must not add domain fields or accept unknown operation fields.

The package contains no store, model call, transport, approval decision, or executor. Tenant and actor values are accepted only after adapters compare them with trusted execution context.
