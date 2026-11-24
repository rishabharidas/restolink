# AGENTS.md - Backend Guidelines (`restolink`)

## 1. Role & Stack
- **Role:** High-performance, multi-tenant core REST & WebSocket API service.
- **Tech Stack:** Go 1.23, `github.com/go-chi/chi/v5`, `github.com/jackc/pgx/v5`, `github.com/redis/go-redis/v9`, `github.com/gorilla/websocket`.

## 2. Key Architectural Directives
1. **Mandatory Tenant Scoping:**
   - Every database query affecting tenant data MUST include `tenant_id = $x`.
   - Never trust client-supplied tenant identifiers without context verification.
2. **Order State Machine Integrity:**
   - Always validate status transitions against `service.ValidateOrderTransition(current, target)`.
   - Disallow skipping states or reversing from terminal states (`COMPLETED`, `CANCELLED`).
   - Record every transition in `restoapp.order_status_history`.
3. **Structured Errors & Responses:**
   - Never use raw unformatted string errors (`http.Error(...)`).
   - Use `errors.WriteError(w, err)` to return standardized JSON error envelopes.
4. **Role-Based Access Control (RBAC):**
   - Use `middleware.RequireRole(...)` to protect sensitive endpoints.
   - Ensure the user's token tenant matches the request's active tenant (unless superadmin).
5. **Quality Verification:**
   - Always run `go vet ./...` and `go test -v ./...` before marking any backend change complete.
