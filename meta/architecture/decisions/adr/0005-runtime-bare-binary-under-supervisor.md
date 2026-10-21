# ADR-0005: Runtime of the single binary

**Status:** Accepted  
**Date:** 2026-10-21  
**Deciders:** @roman.miakotin  
**Revised:** —  
**Migration:** —  
**Pattern:** —  
**API-Posture:** —

## Context

The platform baseline needs an explicit runtime. The design brief fixes one binary configured through environment variables (CON-005) with a pure-Go SQLite store (CON-003) that runs behind the studio's own gateway (CON-006). It does not say where the binary runs, how it is supervised and restarted, or how its environment variables and the SQLite file are provided. The choice decides how the database file is persisted and whether more than one process can ever touch it, and SQLite allows a single writer process. No availability or scale target exists yet (DEC-007 is open), and nothing tells us what infrastructure the studio operates.

## Decision

We will ship the statically linked Go binary and run it on a host under an OS supervisor such as systemd, with an environment file supplying the configuration and the SQLite database on a local disk path given by an environment variable. This satisfies CON-003 and CON-005 with the fewest moving parts: a pure-Go build yields one static binary with no runtime dependencies, the supervisor environment file is the native way to supply environment variables, and the database file cannot be lost to an unmounted volume. The recommended default was accepted as stated.

## Alternatives considered

### bare_binary_supervised (chosen)
The binary under an OS supervisor, an environment file, a local SQLite path. No image build, no registry, no orchestration, which suits a service with no stated scale or availability target.

### container_image
A minimal container image run on the studio's container platform, with a persistent volume for the SQLite file. Portable and immutable, but it adds an image build and registry to delivery, loses all bookings if the volume mount is missing on restart, and must be pinned to a single replica to respect SQLite's single-writer model. Rejected for the first release because nothing in the brief calls for a container platform.

### compose_on_vm
The container image through docker-compose on a VM. Convenient for demos, but compose has nothing to compose here: one service and no separate database (CON-003). It carries both the container-volume risk and the VM provisioning cost. Rejected.

## Consequences

### Positive
- One static artifact with no runtime dependencies, deployed by copying a file.
- Configuration through a supervisor environment file matches CON-005 directly.
- The SQLite file lives on a plain local disk, with no volume mapping to get wrong.

### Negative
- Host provisioning, the supervisor unit and upgrades are manual and host-specific.
- Portability is lower if the studio's gateway platform turns out to be container-based; the decision would then be revisited.

### Neutral
- The service must be kept to a single instance, because SQLite allows one writer process.
- The database path and the other settings are supplied as environment variables and need documenting.

## References

- DEC-021 (resolved by this ADR)
- CTX-001 (the only context)
- CON-003, CON-005, CON-006
- NFR-002

## History

- 2026-10-21: Created — bare binary under an OS supervisor accepted as the recommended default for the runtime axis.
