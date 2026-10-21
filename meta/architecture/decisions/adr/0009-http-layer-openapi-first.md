# ADR-0009: HTTP layer and API contract

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** spec-required

## Context

The service is a JSON API over HTTP built on the standard library `net/http` (CON-002), with errors returned as a JSON object with a stable `code` field (CON-004). Its only caller is the studio's frontend, reached through the studio's gateway (EXT-001), which is a separate consumer that builds against the API and relies on a stable error contract. The engineering standards require the OpenAPI specification to be written first, with types and router generated from it, and allow an exemption only when no separate consumer exists. The brief leaves open whether handlers are hand-written or generated from a contract, and the generated route is a looser reading of "standard library for the HTTP layer".

## Decision

We will write an OpenAPI document as the contract and generate the types and the server interface from it, with a generator that targets the standard `net/http` and mounts the result on `http.ServeMux`. This satisfies CON-002 because the HTTP layer remains `net/http`, and it satisfies CON-004 because the stable error codes are declared once in the specification. The studio, as a separate consumer, therefore receives a published contract and drift between specification and code is caught at build time.

## Alternatives considered

### openapi_first_generated_stdlib (chosen)
A contract-first OpenAPI document with generated types and a standard-library server interface, for example with oapi-codegen. Gives a published contract and build-time drift detection, at the cost of a generator and generated code in the build, and a looser reading of CON-002 because generated code is a dependency beyond the standard library.

### stdlib_servemux_handwritten
Hand-written handlers on `http.ServeMux`, with JSON encoding and the error shape defined in Go. The literal reading of CON-002, with no generation and a small surface. Rejected because there is no machine-readable contract for the studio's frontend, validation and error-code consistency would rest on convention and tests alone, and it conflicts with the project's contract-first standard when a separate consumer exists.

## Consequences

### Positive
- The studio receives a published, machine-readable contract.
- Specification and code cannot drift silently, and error codes are declared in one place.
- Request validation can be generated from the specification.

### Negative
- A generator and generated code become part of the build.
- CON-002 is read as "net/http serves the API", not "no dependency beyond the standard library".

### Neutral
- The OpenAPI document becomes a reviewed artifact, and each endpoint is tested against it.
- The API posture of the project is `spec-required`, because a separate consumer exists.

## References

- DEC-025 (resolved by this ADR)
- CON-002, CON-004, EXT-001
- ADR-0001 (the mismatch error code is declared in the specification)
- CTX-001

## History

- 2026-10-21: Created — OpenAPI-first contract with generated standard-library server code; API posture spec-required.
