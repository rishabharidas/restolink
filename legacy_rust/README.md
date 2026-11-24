# Legacy Rust Prototype Archive

> **STATUS: DEPRECATED & ARCHIVED**

The Rust (Rocket v0.5) prototype contained in this directory was the initial experimentation artifact for the Resto Platform backend.

In accordance with **Architecture Decision Record 001 (`docs/ARCHITECTURE_DECISIONS.md#adr-001`)**, the Resto Platform backend has been completely migrated to a high-throughput, modular **Go 1.23 service (`restolink/cmd/api`)** using Chi, `pgx/v5`, Redis, and Gorilla WebSockets.

This directory is preserved solely for historical audit purposes and is not included in build or deployment pipelines.
