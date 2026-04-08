# Aceryx Feature Specifications — Specs 020–036

**Version:** 1.0
**Date:** 2 April 2026
**Status:** Implementation-ready specifications
**Author:** Neural Chilli Ltd
**Reference:** aceryx-design-thinking-v0.6.1.md

---

## How to Read These Specs

Each spec follows the same structure:

- **Summary** — what this feature is and why it exists.
- **Dependencies** — which specs must be implemented first.
- **Data Model** — database tables, Go types, or YAML schemas.
- **API Surface** — HTTP endpoints, internal interfaces, or CLI commands.
- **Behaviour** — detailed rules governing the feature.
- **BDD Scenarios** — Gherkin-format acceptance tests that define done. These are the implementation contract — if all scenarios pass, the feature is complete.

Specs are ordered by dependency, not by number. The implementation sequence is: 024 → 025 → 026 → 020 → 021 → 022 → 023 → 027 → 028 → 029 → 030 → 031 → 032 → 033 → 034 → 035 → 036.

---

# Plugin Foundation

---

## Spec 024 — Plugin Runtime

### Summary

The plugin runtime is the Wazero-based WebAssembly execution environment that loads, compiles, manages, and invokes WASM plugin modules within the Aceryx process. It provides the host function interface through which plugins interact with Aceryx capabilities (HTTP, case data, vault, secrets, logging, events). It is the foundation on which all plugin-based features are built.

### Dependencies

- Spec 001 (Postgres Schema) — for audit logging of plugin invocations.
- Spec 008 (RBAC) — for tenant-scoped secret access and permission checks.

### Data Model

```sql
-- Plugin registry: loaded plugins and their metadata
CREATE TABLE plugins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id       TEXT NOT NULL,                  -- manifest id field
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('step', 'trigger')),
    category        TEXT NOT NULL,
    licence_tier    TEXT NOT NULL CHECK (licence_tier IN ('open_source', 'commercial')),
    maturity_tier   TEXT NOT NULL CHECK (maturity_tier IN ('core', 'certified', 'community', 'generated')),
    manifest_hash   TEXT NOT NULL,                 -- SHA-256 of manifest.yaml
    wasm_hash       TEXT NOT NULL,                 -- SHA-256 of plugin.wasm
    is_latest       BOOLEAN NOT NULL DEFAULT TRUE, -- latest version flag for unpinned lookups
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'error')),
    error_message   TEXT,
    loaded_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata        JSONB NOT NULL DEFAULT '{}'    -- full manifest as JSON
);

-- Multiple versions of the same plugin can coexist for version pinning
CREATE UNIQUE INDEX idx_plugins_id_version ON plugins (plugin_id, version);
-- Fast lookup for unpinned workflows (latest version)
CREATE UNIQUE INDEX idx_plugins_latest ON plugins (plugin_id) WHERE is_latest = TRUE;

-- Plugin invocation audit log
CREATE TABLE plugin_invocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    plugin_id       TEXT NOT NULL,
    plugin_version  TEXT NOT NULL,                 -- explicit version for audit queries
    wasm_hash       TEXT NOT NULL,
    case_id         UUID,
    step_id         TEXT,
    invocation_type TEXT NOT NULL CHECK (invocation_type IN ('step_execute', 'trigger_event')),
    input_hash      TEXT NOT NULL,                 -- SHA-256 of input payload
    output_hash     TEXT,                          -- SHA-256 of output payload
    duration_ms     INTEGER NOT NULL,
    host_calls      JSONB NOT NULL DEFAULT '[]',   -- log of host function calls
    status          TEXT NOT NULL CHECK (status IN ('success', 'error', 'timeout')),
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_plugin_invocations_tenant ON plugin_invocations (tenant_id, created_at DESC);
CREATE INDEX idx_plugin_invocations_case ON plugin_invocations (case_id);
CREATE INDEX idx_plugin_invocations_plugin ON plugin_invocations (plugin_id, created_at DESC);
```

```go
// Plugin represents a loaded WASM module
type Plugin struct {
    ID             string
    Name           string
    Version        string
    Type           PluginType // StepPlugin | TriggerPlugin
    Category       string
    LicenceTier    string
    MaturityTier   string
    Manifest       PluginManifest
    Module         wazero.CompiledModule
    WASMHash       string
    ManifestHash   string
    Status         PluginStatus
}

type PluginType string
const (
    StepPlugin    PluginType = "step"
    TriggerPlugin PluginType = "trigger"
)

type PluginStatus string
const (
    PluginActive   PluginStatus = "active"
    PluginDisabled PluginStatus = "disabled"
    PluginError    PluginStatus = "error"
)

// HostFunctions is the interface injected into every WASM module
type HostFunctions interface {
    // Low-level HTTP — escape hatch for APIs not in the connector catalogue
    HTTPRequest(method, url string, headers map[string]string, body []byte, timeoutMS int) (HTTPResponse, error)

    // High-level connector invocation — structured call to a registered connector
    // Plugins should prefer this over raw HTTP for any connector in the catalogue.
    // The host handles auth, retry, rate limiting, and error normalisation.
    CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error)

    // Case data (read/write within the current case)
    CaseGet(path string) ([]byte, error)
    CaseSet(path string, value []byte) error

    // Vault (document storage)
    VaultRead(documentID string) ([]byte, error)
    VaultWrite(filename, contentType string, data []byte) (string, error)

    // Secrets (from the tenant's secret store) — returns error if key does not exist
    SecretGet(key string) (string, error)

    // Logging
    Log(level, message string)

    // Events (for trigger plugins)
    CreateCase(caseType string, data []byte) (string, error)
    EmitEvent(eventType string, payload []byte) error

    // Driver-mediated I/O (for trigger plugins)
    // QueueConsume blocks until a message is available. Returns message bytes,
    // metadata headers, and a message_id used for explicit acknowledgement.
    QueueConsume(driverID string, config []byte, topic string) (message []byte, metadata map[string]string, messageID string, err error)
    QueueAck(driverID string, messageID string) error
    FileWatch(driverID string, config []byte, path string) (FileEvent, error)
}

// StepResult is returned by step plugin Execute
type StepResult struct {
    Status  string          `json:"status"`  // "ok" or "error"
    Output  json.RawMessage `json:"output,omitempty"`
    Error   string          `json:"error,omitempty"`
}
```

### API Surface

```go
// PluginRef identifies a plugin, optionally at a specific version.
// If Version is empty, the runtime resolves to the latest loaded version (is_latest=true).
// Workflows store refs as "slack" (latest) or "slack@1.0.0" (pinned).
type PluginRef struct {
    ID      string // plugin_id from manifest
    Version string // semver, empty = latest
}

func ParsePluginRef(s string) PluginRef {
    // "slack@1.0.0" → {ID: "slack", Version: "1.0.0"}
    // "slack"        → {ID: "slack", Version: ""}
}

// PluginRuntime manages the lifecycle of all WASM plugins
type PluginRuntime interface {
    // LoadAll scans the plugins directory and loads all valid plugins
    LoadAll(pluginsDir string, licence LicenceKey) error

    // Load loads a single plugin from a directory
    Load(pluginDir string, licence LicenceKey) (*Plugin, error)

    // Unload removes a specific plugin version from the runtime.
    // If version is empty, unloads all versions.
    Unload(ref PluginRef) error

    // Reload hot-reloads a plugin (stop trigger if running, recompile, restart).
    // If ref has a version, reloads that specific version. If empty, reloads latest.
    Reload(ref PluginRef) error

    // ExecuteStep invokes a step plugin's Execute function.
    // Resolution: if ref has a version, uses that exact version. If empty, uses latest.
    ExecuteStep(ctx context.Context, ref PluginRef, input StepInput) (StepResult, error)

    // StartTrigger begins a trigger plugin's event loop.
    // Triggers always pin to the version loaded at start time.
    StartTrigger(ref PluginRef, config TriggerConfig) error

    // StopTrigger gracefully stops a trigger plugin
    StopTrigger(ref PluginRef) error

    // List returns all loaded plugins (all versions)
    List() []*Plugin

    // Get returns a specific plugin. If ref has no version, returns the latest.
    Get(ref PluginRef) (*Plugin, error)

    // ListVersions returns all loaded versions of a specific plugin
    ListVersions(pluginID string) ([]*Plugin, error)
}
```

```
# REST API endpoints (admin)
GET    /api/v1/admin/plugins              — list all loaded plugins
GET    /api/v1/admin/plugins/:id          — get plugin details
POST   /api/v1/admin/plugins/:id/reload   — hot-reload a plugin
POST   /api/v1/admin/plugins/:id/disable  — disable a plugin
POST   /api/v1/admin/plugins/:id/enable   — enable a plugin
GET    /api/v1/admin/plugins/:id/invocations — invocation audit log
```

### Behaviour

1. **Startup scanning.** On process start, the runtime scans the configured plugins directory (default `/etc/aceryx/plugins/`). Each subdirectory containing a `manifest.yaml` and `plugin.wasm` is a candidate plugin.

2. **Manifest validation.** The manifest is parsed and validated: required fields present, type is valid, version is semver, min/max host version compatibility is checked against the running Aceryx version.

3. **Licence validation.** If `licence_tier` is `commercial`, the runtime checks the installed licence key for the plugin ID or a wildcard commercial grant. If the licence does not permit the plugin, it is skipped with a warning log (not an error — the system starts without it).

4. **WASM compilation.** The `.wasm` file is compiled via Wazero's `CompileModule`. Compilation happens once at startup; the compiled module is reused for all invocations. Compilation errors are logged and the plugin status is set to `error`.

5. **Host function injection.** Before instantiation, the runtime registers all host functions into the WASM module's import namespace. Host functions are scoped to the invoking tenant — `host_secret_get` only returns secrets for the current tenant, `host_case_get/set` only operates on the current case.

6. **Step plugin invocation.** For each invocation, the runtime creates a new WASM module instance (lightweight — shared compiled code, isolated memory), injects tenant-scoped host functions, serialises the input as JSON into WASM linear memory, calls the exported `Execute` function, and reads the result from linear memory. The instance is destroyed after the call.

7. **Trigger plugin lifecycle.** Trigger plugins are instantiated once and run in a dedicated goroutine. The runtime calls the exported `Start` function. The plugin runs its event loop, calling host functions (`host_create_case`, `host_emit_event`) when events arrive. The runtime manages graceful shutdown by calling the exported `Stop` function and waiting up to 30 seconds for the goroutine to exit.

8. **Resource limits.** Each plugin invocation has configurable limits: maximum execution time (default 30s for steps, unlimited for triggers), maximum memory (default 64MB), maximum HTTP response body size (default 10MB). Exceeding limits terminates the invocation with an error.

9. **Host function auditing.** Host function calls are logged in the `host_calls` JSONB array of the invocation record. Three audit modes are available, configurable per plugin or globally:
   - **full** — every host function call with arguments (secrets redacted), result status, and duration. Use for debugging and flagged workflows only.
   - **summary** (default) — function name, call count, total duration, and error count. No individual call arguments.
   - **sampled** — logs every Nth call (configurable, default N=10) with full detail, plus summary for all calls.
   Audit mode is set in the plugin manifest (`audit.host_calls.mode`) or overridden at the workflow/tenant level. Maximum entries per invocation: 50 (configurable). This prevents JSONB bloat under high-throughput plugin usage.

10. **Hot reload.** On SIGHUP or API call, the runtime re-reads the manifest and WASM for the specified plugin. For step plugins, the new module takes effect on the next invocation. For trigger plugins, the current instance is stopped, the new module is compiled, and the trigger is restarted. If the new module fails to compile, the old module remains active. On manifest changes, a schema compatibility check runs: if properties have been removed or renamed, the runtime warns about workflows referencing the old properties. The reload is not blocked — but affected workflows are flagged in the admin UI.

11. **HTTP host function.** `host_http_request` routes through a shared `http.Client` with connection pooling. Per-tenant domain allowlists are enforced — if the tenant has configured allowed domains, requests to other domains are rejected. Requests to localhost, private IP ranges (10.x, 172.16–31.x, 192.168.x), and link-local addresses are always blocked. The request timeout is the minimum of the plugin-specified timeout and the system maximum (default 60s).

12. **CallConnector host function.** `host_call_connector` invokes another registered connector by ID and operation name (e.g. `CallConnector("companies-house", "lookup", {"company_number": "12345"})`). The host resolves the connector, handles authentication, retry, rate limiting, and error normalisation. The plugin receives a structured result map, not raw HTTP. Plugins should prefer `CallConnector` over raw HTTP for any connector in the Aceryx catalogue. `host_http_request` remains as the escape hatch for APIs not in the catalogue.

13. **Plugin version pinning.** Workflows reference plugins with optional version pins: `plugin: slack` (latest) or `plugin: slack@1.0.0` (pinned). When a pinned plugin is reloaded with a new version, workflows pinned to the old version continue using the old module until explicitly updated. The runtime can hold multiple compiled versions of the same plugin simultaneously. Unpinned workflows always use the latest loaded version.

14. **Error isolation.** A panic or trap in a WASM module cannot crash the Aceryx process. Wazero catches WASM traps and returns them as Go errors. The runtime wraps all invocations in recover() as a secondary safety net.

### BDD Scenarios

```gherkin
Feature: Plugin Runtime

  Scenario: Load a valid step plugin on startup
    Given a plugins directory containing "companies-house" with a valid manifest and WASM
    And the licence key permits "companies-house"
    When the plugin runtime starts
    Then the plugin "companies-house" is registered with status "active"
    And the plugin appears in the step palette under category "Financial Services"

  Scenario: Skip commercial plugin without licence
    Given a plugins directory containing "experian" with licence_tier "commercial"
    And the licence key does not include "experian"
    When the plugin runtime starts
    Then the plugin "experian" is not loaded
    And a warning is logged: "Plugin experian requires commercial licence"

  Scenario: Handle invalid WASM gracefully
    Given a plugins directory containing "broken-plugin" with a corrupt WASM file
    When the plugin runtime starts
    Then the plugin "broken-plugin" is registered with status "error"
    And the error_message contains "compilation failed"
    And all other plugins load successfully

  Scenario: Execute a step plugin successfully
    Given the plugin "companies-house" is loaded and active
    And a case with data {"company_number": "12345678"}
    When the engine invokes plugin "companies-house" for the case
    Then the plugin receives the case data as input
    And the plugin calls host_http_request to the Companies House API
    And the plugin returns status "ok" with company data
    And the result is merged into case data at the configured output path
    And a plugin_invocations record is created with status "success"

  Scenario: Step plugin timeout
    Given the plugin "slow-plugin" is loaded with a 30-second timeout
    When the engine invokes "slow-plugin" and it runs for 31 seconds
    Then the invocation is terminated
    And the step result has status "error" with message containing "timeout"
    And a plugin_invocations record is created with status "timeout"

  Scenario: Host function tenant isolation
    Given tenant A has a secret "api_key" = "key-A"
    And tenant B has a secret "api_key" = "key-B"
    When a plugin executes in the context of tenant A and calls host_secret_get("api_key")
    Then the plugin receives "key-A"
    And the plugin cannot access tenant B's secrets

  Scenario: Block requests to private IP ranges
    Given a plugin calls host_http_request with URL "http://192.168.1.1/internal"
    Then the host function returns an error "request to private IP range blocked"
    And no HTTP request is made

  Scenario: Hot-reload a step plugin
    Given the plugin "slack" is loaded at version "1.0.0"
    When the "slack" plugin directory is updated with version "1.1.0"
    And a reload is triggered for "slack"
    Then the new WASM is compiled successfully
    And subsequent invocations use the "1.1.0" module
    And the plugins table reflects version "1.1.0"

  Scenario: Hot-reload a trigger plugin
    Given the trigger plugin "kafka-consumer" is running
    When a reload is triggered for "kafka-consumer"
    Then the current trigger is stopped gracefully
    And the new WASM is compiled
    And the trigger is restarted with the new module

  Scenario: Failed hot-reload preserves old module
    Given the plugin "slack" is loaded at version "1.0.0"
    When the "slack" plugin directory is updated with a corrupt WASM
    And a reload is triggered for "slack"
    Then the reload fails with a compilation error
    And the "1.0.0" module remains active
    And an error is logged

  Scenario: Plugin invocation audit trail — summary mode (default)
    Given the plugin "open-banking" is loaded with audit mode "summary"
    When the plugin executes and calls host_http_request twice and host_case_set once
    Then the plugin_invocations record contains host_calls summary:
      | function | call_count | total_duration_ms | errors |
      | host_http_request | 2 | 412 | 0 |
      | host_case_set | 1 | 1 | 0 |
    And individual call arguments are not recorded

  Scenario: Plugin invocation audit trail — full mode
    Given the plugin "open-banking" is loaded with audit mode "full"
    When the plugin executes and calls host_http_request twice
    Then the plugin_invocations record contains host_calls with 2 detailed entries
    And each entry records function name, arguments (secrets redacted), duration, and status

  Scenario: Audit mode capped at max entries
    Given a plugin with audit mode "full" and max_entries = 50
    When the plugin makes 100 host function calls
    Then only the first 50 calls are logged in detail
    And a summary of remaining 50 calls is appended

  Scenario: Memory limit enforcement
    Given a plugin attempts to allocate 128MB of memory (limit is 64MB)
    Then the WASM module traps with an out-of-memory error
    And the Aceryx process is unaffected
    And the invocation is recorded with status "error"

  Scenario: CallConnector for structured integration
    Given the plugin "loan-assessment" needs to call the "companies-house" connector
    When the plugin calls CallConnector("companies-house", "lookup", {"company_number": "12345678"})
    Then the host resolves the "companies-house" connector
    And handles authentication, retry, and rate limiting
    And returns structured company data as a map
    And the plugin does not need to know the Companies House API URL or auth method

  Scenario: CallConnector for unregistered connector
    Given the plugin calls CallConnector("nonexistent", "lookup", {})
    Then the host returns an error "connector not found: nonexistent"

  Scenario: Plugin version pinning — pinned workflow
    Given the plugin "slack" is loaded at versions "1.0.0" and "1.1.0"
    And a workflow references "plugin: slack@1.0.0"
    When the workflow executes
    Then the "1.0.0" module is used, not "1.1.0"

  Scenario: Plugin version pinning — unpinned workflow
    Given the plugin "slack" is loaded at versions "1.0.0" and "1.1.0"
    And a workflow references "plugin: slack" (no version pin)
    When the workflow executes
    Then the latest version "1.1.0" is used

  Scenario: Reload with property schema change
    Given the plugin "slack" v1.0.0 has property "channel" (required)
    When v1.1.0 renames the property to "channel_id"
    And a reload is triggered
    Then the reload succeeds
    And workflows referencing property "channel" are flagged in the admin UI
    And a warning is logged: "plugin slack@1.1.0: property 'channel' removed, 3 workflows affected"

  Scenario: List plugins via API
    Given 5 plugins are loaded (3 active, 1 disabled, 1 error)
    When GET /api/v1/admin/plugins is called
    Then the response contains 5 plugins with their status, version, and category
    And plugins are sorted by category then name

  Scenario: Disable and re-enable a plugin
    Given the plugin "salesforce" is active
    When POST /api/v1/admin/plugins/salesforce/disable is called
    Then the plugin status changes to "disabled"
    And it is removed from the step palette
    When POST /api/v1/admin/plugins/salesforce/enable is called
    Then the plugin status changes to "active"
    And it reappears in the step palette
```

---

## Spec 025 — Plugin SDK

### Summary

The Plugin SDK provides libraries for Go/TinyGo and Rust that abstract the WASM host function interface into idiomatic language constructs. It also provides a CLI scaffolder (`aceryx plugin init`) that generates a new plugin project with manifest, boilerplate code, and build instructions. The SDK is the primary developer-facing surface of the plugin system — its quality and documentation directly determine ecosystem growth.

### Dependencies

- Spec 024 (Plugin Runtime) — defines the host function interface the SDK wraps.

### Data Model

No database tables. The SDK is a set of libraries and CLI tools.

```go
// Go/TinyGo SDK — github.com/neural-chilli/aceryx-plugin-sdk-go/sdk

// Context is the primary interface for plugin authors
type Context interface {
    // HTTP makes a raw HTTP request via the host — escape hatch for APIs
    // not in the Aceryx connector catalogue. Prefer CallConnector where possible.
    // Returns (Response, error) — error covers transport failures, blocked domains,
    // timeouts, and TLS errors. A non-2xx HTTP status is NOT an error; check resp.Status.
    HTTP(req Request) (Response, error)

    // CallConnector invokes a registered Aceryx connector by ID.
    // The host handles auth, retry, rate limiting, and error normalisation.
    // Returns structured result data. Preferred over raw HTTP.
    CallConnector(connectorID, operation string, input map[string]any) (map[string]any, error)

    // CaseGet reads a value from case data by JSON path
    CaseGet(path string) (string, error)

    // CaseSet writes a value to case data by JSON path
    CaseSet(path string, value interface{}) error

    // VaultRead reads a document from the vault
    VaultRead(documentID string) ([]byte, error)

    // VaultWrite stores a document in the vault
    VaultWrite(filename, contentType string, data []byte) (string, error)

    // Secret reads a secret from the tenant's secret store.
    // Returns an error if the key does not exist (not an empty string).
    Secret(key string) (string, error)

    // Log writes a log message
    Log(level LogLevel, msg string, args ...interface{})

    // Config reads a plugin configuration value from the manifest properties
    Config(key string) string
}

// Request for host_http_request
type Request struct {
    Method   string
    URL      string
    Headers  map[string]string
    Body     []byte
    Timeout  int // milliseconds, 0 = default
}

// Response from host_http_request
type Response struct {
    Status     int
    StatusText string
    Headers    map[string]string
    Body       []byte
}

func (r Response) JSON() map[string]interface{} { ... }
func (r Response) Text() string { ... }

// Result types
type Result struct { ... }
func OK() Result { ... }
func OKWithOutput(data interface{}) Result { ... }
func Error(msg string) Result { ... }
func ErrorWithCode(code, msg string) Result { ... }

// TriggerContext extends Context for trigger plugins with event emission
// and driver-mediated I/O for queues, files, and polling.
type TriggerContext interface {
    Context

    // Case/event creation
    CreateCase(caseType string, data interface{}) (string, error)
    EmitEvent(eventType string, payload interface{}) error

    // QueueConsume blocks until a message is available on the given topic.
    // Returns message body, metadata headers, and a messageID for acknowledgement.
    // For host_managed state with at_least_once delivery, the host acks after
    // pipeline success — plugins should NOT call QueueAck directly.
    // For plugin_managed state, the plugin must call QueueAck explicitly.
    QueueConsume(driverID, topic string) (message []byte, metadata map[string]string, messageID string, err error)

    // QueueAck explicitly acknowledges a message. Only used when
    // trigger_contract.state = "plugin_managed". For host_managed state,
    // calling QueueAck returns an error.
    QueueAck(driverID, messageID string) error

    // FileWatch blocks until a file change is detected on the configured path.
    // Returns the file path, event type (created/modified/deleted), and metadata.
    FileWatch(driverID, path string) (event FileEvent, err error)

    // PollHTTP is SDK sugar over HTTP + sleep. NOT a separate host function.
    // Internally calls ctx.HTTP() then sleeps for intervalMS before returning.
    // The plugin controls the loop; PollHTTP handles one request-and-wait cycle.
    // Audit and rate limiting apply through the underlying HTTP host function.
    PollHTTP(url string, headers map[string]string, intervalMS int) (Response, error)
}
```

```rust
// Rust SDK — aceryx-plugin-sdk crate

pub trait Context {
    fn http(&self, req: Request) -> Result<Response>;
    fn call_connector(&self, connector_id: &str, operation: &str, input: impl Serialize) -> Result<Value>;
    fn case_get(&self, path: &str) -> Result<Value>;
    fn case_set(&self, path: &str, value: impl Serialize) -> Result<()>;
    fn vault_read(&self, document_id: &str) -> Result<Vec<u8>>;
    fn vault_write(&self, filename: &str, content_type: &str, data: &[u8]) -> Result<String>;
    fn secret(&self, key: &str) -> Result<String>;
    fn log(&self, level: LogLevel, msg: &str);
    fn config(&self, key: &str) -> Option<String>;
}

pub trait TriggerContext: Context {
    fn create_case(&self, case_type: &str, data: impl Serialize) -> Result<String>;
    fn emit_event(&self, event_type: &str, payload: impl Serialize) -> Result<()>;
    fn queue_consume(&self, driver_id: &str, topic: &str) -> Result<QueueMessage>;
    fn queue_ack(&self, driver_id: &str, message_id: &str) -> Result<()>;
    fn file_watch(&self, driver_id: &str, path: &str) -> Result<FileEvent>;
    /// SDK sugar over http() + sleep. Not a separate host function.
    fn poll_http(&self, url: &str, headers: &HashMap<String, String>, interval_ms: u32) -> Result<Response>;
}

// Macro for step plugins
#[aceryx_plugin]
fn execute(ctx: &mut impl Context) -> Result<()> { ... }

// Macro for trigger plugins
#[aceryx_trigger]
fn start(ctx: &mut impl TriggerContext) -> Result<()> { ... }
```

### CLI — `aceryx plugin init`

```
$ aceryx plugin init --lang=go --type=step --name=my-connector
Creating plugin: my-connector
  my-connector/
    manifest.yaml       # pre-filled template
    main.go             # step plugin boilerplate
    main_test.go        # test harness with mock context
    Makefile             # build targets: build, test, clean
    README.md            # documentation template
    .gitignore

Build with: cd my-connector && make build
Test with:  cd my-connector && make test
Output:     my-connector/plugin.wasm

$ aceryx plugin init --lang=rust --type=trigger --name=kafka-ingest
Creating plugin: kafka-ingest
  kafka-ingest/
    manifest.yaml
    Cargo.toml
    src/lib.rs           # trigger plugin boilerplate
    src/tests.rs
    Makefile
    README.md
    .gitignore
```

### Behaviour

1. **SDK versioning.** The SDK version is pinned to a host function ABI version (e.g. ABI v1). Multiple SDK versions can exist (e.g. sdk-go v1.2, v1.3) as long as they target the same ABI. The ABI version is checked at plugin load time.

2. **Serialisation.** All data crossing the WASM boundary is serialised as JSON. The SDK handles serialisation/deserialisation transparently — plugin authors work with native types, not byte buffers.

3. **Error handling.** Go SDK uses standard Go error returns. Rust SDK uses `Result<T>`. Errors from host functions are propagated as SDK-level errors with descriptive messages. The SDK never panics — all errors are returned.

4. **Testing.** Both SDKs ship with a mock `Context` implementation that records all host function calls and allows injecting responses. Plugin authors write unit tests against the mock without needing a running Aceryx instance.

5. **Build toolchain.** The Go SDK requires TinyGo for WASM compilation. The Rust SDK requires `wasm32-wasi` target. The scaffolded Makefile includes the correct build commands. The `aceryx plugin build` command wraps the toolchain invocation.

6. **Config access.** The `Config(key)` function reads values from the plugin's configured properties (as set by the user in the builder UI). These are passed to the plugin as part of the invocation input, not read from disk.

7. **Logging.** `Log()` calls are forwarded to the Aceryx logger with the plugin ID as a prefix. Log levels: debug, info, warn, error. Log output appears in Aceryx's standard log stream.

### BDD Scenarios

```gherkin
Feature: Plugin SDK

  Scenario: Scaffold a Go step plugin
    When "aceryx plugin init --lang=go --type=step --name=my-plugin" is run
    Then a directory "my-plugin" is created
    And it contains manifest.yaml with type "step" and name "my-plugin"
    And it contains main.go with a stub Execute function
    And it contains a Makefile with "build" and "test" targets
    And "make build" produces plugin.wasm

  Scenario: Scaffold a Rust trigger plugin
    When "aceryx plugin init --lang=rust --type=trigger --name=my-trigger" is run
    Then a directory "my-trigger" is created
    And it contains manifest.yaml with type "trigger"
    And it contains src/lib.rs with a stub start function using #[aceryx_trigger]
    And "make build" produces plugin.wasm

  Scenario: Go SDK HTTP request — success
    Given a Go step plugin that calls ctx.HTTP with GET "https://api.example.com/data"
    When the plugin is executed and the API returns 200
    Then ctx.HTTP returns (Response, nil)
    And resp.Status is 200
    And resp.JSON() returns a parsed map

  Scenario: Go SDK HTTP request — transport error
    Given a Go step plugin that calls ctx.HTTP with GET "https://api.example.com/data"
    When the host cannot connect (DNS failure, timeout, TLS error)
    Then ctx.HTTP returns (Response{}, error) with a descriptive message
    And the plugin can handle the error without inspecting a fake status code

  Scenario: Go SDK HTTP request — blocked domain
    Given a Go step plugin that calls ctx.HTTP with a blocked domain
    Then ctx.HTTP returns (Response{}, error) with message "domain not allowed"

  Scenario: Go SDK HTTP request — non-2xx is not an error
    Given a Go step plugin that calls ctx.HTTP and the API returns 404
    Then ctx.HTTP returns (Response, nil) — no error
    And resp.Status is 404
    And the plugin decides how to handle the 404

  Scenario: Trigger SDK — QueueConsume
    Given a Go trigger plugin that calls ctx.QueueConsume("kafka", "loan-applications")
    When a message arrives on the topic
    Then QueueConsume returns (message, metadata, messageID, nil)
    And the plugin can parse the message and call ctx.CreateCase

  Scenario: Trigger SDK — FileWatch
    Given a Go trigger plugin that calls ctx.FileWatch("sftp", "/incoming")
    When a new file appears on the SFTP server
    Then FileWatch returns a FileEvent with path, event type, and metadata

  Scenario: Rust SDK case data access
    Given a Rust step plugin that calls ctx.case_get("applicant.name")
    And the case data contains {"applicant": {"name": "Alice"}}
    When the plugin is executed
    Then ctx.case_get returns Ok(Value::String("Alice"))

  Scenario: SDK Secret — missing key returns error
    Given a Go plugin that calls ctx.Secret("nonexistent")
    And the secret does not exist
    When the plugin is executed
    Then ctx.Secret returns ("", error) with message "secret not found: nonexistent"
    And the plugin can distinguish missing from empty

  Scenario: SDK Secret — existing key returns value
    Given a Go plugin that calls ctx.Secret("api_key")
    And the secret "api_key" exists with value "sk-12345"
    When the plugin is executed
    Then ctx.Secret returns ("sk-12345", nil)

  Scenario: SDK CallConnector
    Given a Go plugin that calls ctx.CallConnector("companies-house", "lookup", map)
    And the "companies-house" connector is registered
    When the plugin is executed
    Then the SDK serialises the call to host_call_connector
    And the host resolves the connector, handles auth and retry
    And the result is deserialised into a map[string]any

  Scenario: SDK CallConnector — connector not found
    Given a Go plugin that calls ctx.CallConnector("nonexistent", "lookup", map)
    When the plugin is executed
    Then the call returns an error "connector not found: nonexistent"

  Scenario: Mock context for testing
    Given a Go plugin test using sdk.MockContext
    And the mock has HTTP response configured for "https://api.example.com"
    And the mock has CallConnector response configured for "companies-house"
    When the plugin's Execute function is called with the mock
    Then the mock records the HTTP request arguments
    And the mock records the CallConnector arguments
    And the test can assert on all recorded calls

  Scenario: Config access
    Given a plugin with manifest property "environment" defaulting to "sandbox"
    And the user has configured "environment" = "production"
    When the plugin calls ctx.Config("environment")
    Then it returns "production"

  Scenario: ABI version check
    Given a plugin compiled with SDK ABI v1
    And the Aceryx runtime supports ABI v1 and v2
    When the plugin is loaded
    Then it loads successfully
    Given a plugin compiled with SDK ABI v3
    When the plugin is loaded
    Then it fails with "unsupported ABI version: 3"
```

---

## Spec 026 — Plugin Manifest & Registry

### Summary

The plugin manifest is a YAML file that declares everything the Aceryx host needs to load, display, configure, and licence-gate a plugin. The registry is the in-process catalogue of loaded plugins that powers the builder's step palette, the admin API, and the licence enforcement.

### Dependencies

- Spec 024 (Plugin Runtime) — manifest is consumed by the runtime.
- Spec 009 (Schema-Driven Forms) — property schemas are rendered by the form engine.

### Data Model

```yaml
# manifest.yaml — full schema
id: string                    # unique plugin identifier (lowercase, hyphens)
name: string                  # display name
version: string               # semver
type: step | trigger          # plugin type
category: string              # sidebar category
tier: open_source | commercial # licence tier
maturity: core | certified | community | generated # quality tier
min_host_version: string      # minimum Aceryx version (semver range)
max_host_version: string      # maximum Aceryx version (semver range)

ui:
  icon_svg: string            # inline SVG, max 8KB
  description: string         # one-line description for palette tooltip
  long_description: string    # markdown, for plugin detail page
  properties:                 # schema-driven configuration UI
    - key: string
      label: string
      type: text | secret | select | expression | json | number | boolean
      required: boolean
      default: any
      options: [string]       # for select type
      help_text: string
      validation: string      # regex or expression

host_functions:               # declared host function requirements
  - host_http_request
  - host_secret_get
  - host_log

operational:                  # per-connector operational metadata
  retry_semantics: automatic | manual | none
  transaction_guarantee: exactly_once | at_least_once | best_effort
  idempotent: boolean
  rate_limited: boolean
  rate_limit_config:
    requests_per_second: number
    burst: number

cost:                         # cost signal for AI-assisted workflow generation
  level: free | low | medium | high | very_high
  billing_unit: per_call | per_token | per_record | per_mb | flat
  notes: string               # e.g. "Experian charges per credit check"

audit:                        # audit configuration for this plugin
  host_calls:
    mode: full | summary | sampled   # default: summary
    max_entries: number               # default: 50
    sample_rate: number               # for sampled mode, default: 10

trigger_contract:             # REQUIRED for trigger plugins, rejected for step plugins
  delivery: at_least_once | exactly_once | best_effort
  state: host_managed | plugin_managed
  concurrency: single | parallel
  ordering: ordered | unordered
  checkpoint:
    strategy: per_message | periodic | on_shutdown
    interval_ms: number       # for periodic strategy

trigger_config:               # for trigger plugins only
  polling_interval_ms: number # default polling interval
  configurable_interval: boolean
```

### Behaviour

1. **Manifest parsing.** On load, the runtime parses `manifest.yaml` using lenient parsing: unknown fields are ignored with a warning log (forward-compatible — newer plugins may include fields the current host doesn't understand), while missing required fields cause a load failure. This is explicitly not strict-and-fail; the intent is that a plugin built for a newer Aceryx version can still load on an older host as long as the required fields are present. The parsed manifest is stored in the `plugins` table as JSONB.

2. **ID uniqueness.** Plugin IDs must be globally unique. If two directories contain plugins with the same ID, the one with the higher semver version wins. If versions are equal, load fails with an explicit error.

3. **Version compatibility.** `min_host_version` and `max_host_version` are checked against the running Aceryx binary version. Plugins outside the compatibility range are skipped with a warning.

4. **Icon handling.** The `icon_svg` is validated: must be valid SVG, max 8KB after whitespace trimming. If invalid or missing, a default category icon is used. Icons are cached in memory and served via the builder API.

5. **Property rendering.** The `properties` array drives the builder's properties panel for this plugin. Each property maps to a form field using the schema-driven form renderer (spec 009). The `secret` type renders a password input and stores the value in the tenant's secret store, not in workflow YAML.

6. **Host function validation.** The `host_functions` array declares which host functions the plugin requires. At load time, the runtime verifies all declared functions are available. At invocation time, calls to undeclared host functions are blocked — this prevents plugins from accessing capabilities they didn't declare (defense in depth).

7. **Operational metadata.** Core and Certified plugins must include the `operational` block. Community and Generated plugins may omit it. The metadata is displayed in the builder's properties panel and is available to the AI assistant for intelligent workflow generation (e.g. the assistant knows to add retry logic around a connector that declares `retry_semantics: manual`).

8. **Cost metadata.** The `cost` block signals the expense profile of a connector to the AI assistant and workflow builder. When the AI assistant generates workflows, it uses cost levels to make intelligent choices — preferring a `free` Companies House lookup over a `very_high` Experian credit check when both could satisfy the requirement. The builder displays a cost indicator badge on each step. Cost metadata is advisory, not enforced — it does not block execution.

9. **Trigger contract.** Trigger plugins must include the `trigger_contract` block. Step plugins must not. The contract declares delivery guarantees, state management responsibility, concurrency model, ordering, and checkpointing strategy. The runtime uses this contract to:
   - Configure acknowledgement behaviour (at-least-once: ack after pipeline success; exactly-once: ack within a transaction; best-effort: ack immediately).
   - Manage concurrency (single: one goroutine; parallel: configurable worker pool).
   - Handle state persistence (host_managed: runtime persists offsets/cursors; plugin_managed: plugin handles its own state via host functions).
   Trigger plugins missing the `trigger_contract` block fail manifest validation.

10. **Audit configuration.** The `audit` block configures host function call logging granularity for this plugin. Defaults to `summary` mode. Individual tenants or workflows can override the plugin-level setting. See spec 024 §9 for audit mode details.

11. **Registry queries.** The registry supports filtering by type (step/trigger), category, tier, maturity, cost level, and keyword search on name/description. The builder palette uses this to populate its sidebar.

### BDD Scenarios

```gherkin
Feature: Plugin Manifest & Registry

  Scenario: Valid manifest loads successfully
    Given a plugin directory with a manifest containing all required fields
    And the host version is within min/max range
    When the plugin is loaded
    Then the manifest is parsed and stored in the plugins table
    And the plugin appears in the registry

  Scenario: Missing required field fails load
    Given a manifest missing the "id" field
    When the plugin is loaded
    Then loading fails with error "manifest missing required field: id"
    And the plugin is not registered

  Scenario: Host version outside range
    Given a manifest with min_host_version "2.0.0"
    And the running Aceryx version is "1.5.0"
    When the plugin is loaded
    Then loading is skipped with warning "plugin requires Aceryx >= 2.0.0"

  Scenario: Duplicate plugin IDs resolved by version
    Given plugin directory A contains "slack" at version "1.0.0"
    And plugin directory B contains "slack" at version "1.1.0"
    When both are scanned
    Then version "1.1.0" is loaded
    And version "1.0.0" is skipped with an info log

  Scenario: Property schema renders in builder
    Given a plugin with properties: api_key (secret, required), environment (select: sandbox/production)
    When the plugin step is selected in the builder
    Then the properties panel shows a password field for "API Key" marked required
    And a dropdown for "Environment" with options "sandbox" and "production"

  Scenario: Secret property stored securely
    Given a plugin with a property of type "secret"
    When the user enters a value in the builder
    Then the value is stored in the tenant's secret store
    And the workflow YAML contains a reference, not the plaintext value

  Scenario: Undeclared host function blocked
    Given a plugin manifest declares host_functions: [host_http_request, host_log]
    When the plugin calls host_secret_get at runtime
    Then the call is blocked with error "undeclared host function: host_secret_get"
    And the invocation is logged with the blocked call

  Scenario: Operational metadata displayed
    Given a plugin with operational.retry_semantics = "automatic" and operational.idempotent = true
    When the plugin step is viewed in the builder
    Then the properties panel shows "Retry: Automatic" and "Idempotent: Yes"

  Scenario: Registry filtering
    Given 10 plugins loaded across 3 categories and 2 maturity tiers
    When the builder requests plugins filtered by category "Financial Services"
    Then only plugins in that category are returned
    When filtered by maturity "core"
    Then only core-tier plugins are returned

  Scenario: Icon validation
    Given a manifest with icon_svg containing 12KB of SVG
    When the plugin is loaded
    Then the oversized icon is replaced with the default category icon
    And a warning is logged

  Scenario: Manifest hash change detection
    Given a plugin "slack" loaded with manifest hash "abc123"
    When the manifest file is modified
    And a reload is triggered
    Then the new manifest hash is computed
    And the plugins table is updated with the new hash

  Scenario: Cost metadata displayed in builder
    Given a plugin with cost.level = "very_high" and cost.notes = "Experian charges per credit check"
    When the plugin step is viewed in the builder
    Then the step shows a cost indicator badge "Very High"
    And the tooltip shows "Experian charges per credit check"

  Scenario: AI assistant uses cost metadata
    Given the AI assistant is generating a workflow that needs company verification
    And "companies-house" has cost.level = "free"
    And "experian" has cost.level = "very_high"
    When the assistant selects a connector for basic company lookup
    Then it prefers "companies-house" over "experian"
    And includes a note: "Using Companies House (free) for basic lookup. Add Experian for full credit check if needed."

  Scenario: Trigger plugin missing trigger_contract
    Given a trigger plugin manifest without a trigger_contract block
    When the plugin is loaded
    Then loading fails with error "trigger plugins must include trigger_contract"

  Scenario: Step plugin with trigger_contract rejected
    Given a step plugin manifest that includes a trigger_contract block
    When the plugin is loaded
    Then loading fails with error "step plugins must not include trigger_contract"

  Scenario: Trigger contract — at-least-once delivery
    Given a trigger plugin with trigger_contract.delivery = "at_least_once"
    When the runtime configures the trigger
    Then message acknowledgement is deferred until the pipeline confirms processing
    And the runtime expects the source to redeliver unacknowledged messages

  Scenario: Trigger contract — host-managed state
    Given a trigger plugin with trigger_contract.state = "host_managed"
    When the trigger consumes messages
    Then the runtime persists offsets/cursors after each successful acknowledgement
    And on restart, the trigger resumes from the last persisted offset
```

---

# Core AI & Authoring

---

## Spec 020 — AI Assistant

### Summary

The AI assistant is a page-aware conversational copilot with four authoring modes: Describe (generate workflows from natural language), Refactor (modify existing workflows), Explain (reverse-engineer intent), and Generate Test Cases. Every AI-authored change is presented as a YAML diff before applying, with the diff persisted in the audit trail. The assistant is a commercial-tier feature.

### Dependencies

- Spec 010 (Visual Flow Builder) — the builder is the primary integration surface.
- Spec 002 (Execution Engine) — workflow YAML schema defines valid output.
- Spec 011 (Audit Trail) — diffs are persisted as audit events.
- Spec 026 (Plugin Manifest) — the assistant reads plugin metadata for context.

### Data Model

```sql
CREATE TABLE ai_assistant_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    user_id         UUID NOT NULL REFERENCES users(id),
    page_context    TEXT NOT NULL,         -- 'builder', 'cases', 'inbox', 'admin'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_assistant_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES ai_assistant_sessions(id),
    role            TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content         TEXT NOT NULL,
    mode            TEXT CHECK (mode IN ('describe', 'refactor', 'explain', 'test_generate')),
    yaml_before     TEXT,                  -- workflow YAML before change (for diffs)
    yaml_after      TEXT,                  -- workflow YAML after change
    diff            TEXT,                  -- computed unified diff
    applied         BOOLEAN DEFAULT FALSE, -- whether the user accepted the change
    model_used      TEXT,                  -- LLM model identifier
    tokens_used     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_assistant_diffs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    workflow_id     UUID NOT NULL,
    message_id      UUID NOT NULL REFERENCES ai_assistant_messages(id),
    user_id         UUID NOT NULL REFERENCES users(id),
    prompt          TEXT NOT NULL,          -- user's request
    diff            TEXT NOT NULL,          -- unified diff
    applied         BOOLEAN NOT NULL,       -- accepted or rejected
    applied_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_diffs_workflow ON ai_assistant_diffs (workflow_id, created_at DESC);
```

### Behaviour

1. **Page context providers.** Each page registers a context provider function that assembles relevant state into the system prompt:
   - **Builder:** current workflow YAML, step catalogue (all loaded plugins with properties), validation errors, workflow metadata.
   - **Cases:** case type schema, case data, case timeline, associated workflow.
   - **Inbox:** current task, task form schema, case summary.
   - **Admin:** tenant configuration, plugin list, licence details.

2. **Describe mode.** User provides natural language. The assistant generates complete workflow YAML against the Aceryx schema. Schema validation runs after generation. If validation fails, errors are fed back to the LLM for self-correction (up to 3 attempts). If validation succeeds, the YAML is presented as a full diff against an empty workflow.

3. **Refactor mode.** User describes a modification to an existing workflow. The assistant receives the current YAML and produces modified YAML. The diff between before and after is computed and displayed. The user reviews and approves or rejects.

4. **Explain mode.** The assistant receives workflow YAML and generates a plain-English explanation of what it does: purpose, trigger, steps, decision points, SLAs, escalation paths. No YAML is generated in this mode.

5. **Generate Test Cases mode.** The assistant receives workflow YAML and generates Gherkin-format test scenarios covering happy path, edge cases, error conditions, SLA breaches, and human task paths. Output is presented as a downloadable `.feature` file.

6. **Diff view.** Every YAML change is presented as a unified diff. The diff is rendered in the builder with syntax highlighting: green for additions, red for deletions, grey for context. The user clicks "Apply" or "Reject". Only "Apply" modifies the workflow.

7. **Audit trail.** Every AI interaction is recorded: the prompt, the mode, the YAML before/after, the diff, whether it was applied, and the model used. The `ai_assistant_diffs` table provides a complete history of AI-authored changes per workflow — answerable to "who changed this and why."

8. **Placeholder steps.** When the assistant cannot map a described step to an existing step type or plugin, it inserts a `type: placeholder` step with description, expected inputs, expected outputs. Placeholders are visually distinct (dashed border, warning icon) and block workflow publishing.

9. **Model routing.** The assistant uses the configured LLM provider (OpenAI, Anthropic, or local). The system prompt includes the full Aceryx workflow schema, the plugin catalogue with property schemas, and page-specific context. Model selection is configurable per tenant.

10. **Rate limiting.** AI assistant requests are rate-limited per user (default: 30 requests/minute) and per tenant (default: 200 requests/minute). Rate limit headers are returned in the WebSocket messages.

11. **Static analysis (post-validation).** After schema validation passes, a static analysis pass runs on the generated YAML to catch semantically valid but logically broken workflows. The analysis detects:
    - **Unreachable steps.** Steps that no other step transitions to and that are not the entry point.
    - **Missing output mappings.** Steps that write to a case data path that no downstream step reads from (warning, not error).
    - **Dangling input references.** Steps that read from a case data path that no upstream step or channel populates.
    - **Connector misconfiguration.** Steps referencing a plugin that is not installed, or using properties that don't match the plugin's manifest schema.
    - **Infinite loops.** Cycles in the DAG that have no exit condition (e.g. a retry loop with no max retries).
    - **Cost warnings.** Steps using high-cost connectors (cost.level = "high" or "very_high") without an obvious justification in the workflow context — surfaced as a suggestion, not a block.
    Static analysis findings are returned alongside the generated YAML. Errors (unreachable steps, infinite loops) prevent the diff from being applied. Warnings (missing output mappings, cost) are displayed but do not block. The AI assistant is given the findings and can self-correct in a subsequent attempt.

12. **Cost-aware generation.** When generating workflows, the assistant receives plugin cost metadata from the registry. It prefers lower-cost connectors where multiple options satisfy the requirement. If a high-cost connector is used, the assistant adds a comment explaining why (e.g. "Experian credit check required — Companies House does not provide credit scoring").

### BDD Scenarios

```gherkin
Feature: AI Assistant

  Scenario: Describe mode generates valid YAML
    Given the user is on the Builder page with an empty workflow
    When the user types "When an email arrives with a PDF, extract the data, check the credit score, and if above 650 auto-approve, otherwise send to manual review with a 2-day SLA"
    Then the assistant generates workflow YAML
    And the YAML passes schema validation
    And the diff view shows the complete workflow as additions (green)
    And the YAML includes steps for email trigger, document extraction, credit check, conditional routing, auto-approve, and human review with SLA

  Scenario: Describe mode with self-correction
    Given the assistant generates YAML with an invalid step type
    When schema validation fails
    Then the validation errors are sent back to the LLM
    And the LLM generates corrected YAML (attempt 2)
    And the corrected YAML passes validation

  Scenario: Describe mode exhausts retries
    Given the LLM fails to produce valid YAML after 3 attempts
    Then the assistant displays an error: "I wasn't able to generate a valid workflow. Here's what I tried and the validation errors."
    And the last attempt's YAML and errors are shown for manual fixing

  Scenario: Refactor mode produces diff
    Given a workflow with 5 steps and no retry logic
    When the user types "Add retry with backoff to all API call steps"
    Then the assistant produces modified YAML with retry config on API steps
    And a unified diff is displayed showing only the changed lines
    And unchanged steps are shown as grey context

  Scenario: User applies AI-authored diff
    Given a diff is displayed for a refactor change
    When the user clicks "Apply"
    Then the workflow YAML is updated to the new version
    And the builder re-renders with the changes
    And an ai_assistant_diffs record is created with applied = true

  Scenario: User rejects AI-authored diff
    Given a diff is displayed for a refactor change
    When the user clicks "Reject"
    Then the workflow YAML is unchanged
    And an ai_assistant_diffs record is created with applied = false

  Scenario: Explain mode
    Given a workflow with conditional routing, human tasks, and SLA escalation
    When the user types "Explain this workflow"
    Then the assistant returns a plain-English description
    And the description covers: purpose, trigger, each step's function, decision logic, SLAs, and escalation paths
    And no YAML is generated

  Scenario: Generate test cases
    Given a loan application workflow
    When the user types "Generate test cases"
    Then the assistant produces Gherkin scenarios covering:
      | Category      | Example                                    |
      | Happy path    | Application approved with high credit score |
      | Edge case     | Credit score exactly at threshold           |
      | Error         | Credit check API unavailable                |
      | SLA breach    | Manual review not completed within SLA      |
      | Human task    | Reviewer rejects application                |

  Scenario: Placeholder step for unknown capability
    Given the plugin catalogue does not include a "send-fax" connector
    When the user describes a workflow that includes "send a fax to the broker"
    Then the generated YAML includes a step with type "placeholder"
    And the placeholder has description "Send fax to broker" with expected inputs and outputs
    And the placeholder step renders with a dashed border and warning icon
    And the workflow cannot be published while placeholders exist

  Scenario: Page context — Builder vs Cases
    Given the user opens the assistant on the Builder page
    Then the system prompt includes workflow YAML and the plugin catalogue
    When the user navigates to a Case view and opens the assistant
    Then the system prompt includes case data, case type schema, and timeline
    And the assistant can answer questions about the specific case

  Scenario: Audit trail for AI changes
    Given a user applies 3 AI-authored changes to a workflow over time
    When an auditor queries ai_assistant_diffs for that workflow
    Then 3 records are returned, each with prompt, diff, and applied timestamp
    And the complete history of AI modifications is traceable

  Scenario: Rate limiting
    Given a user sends 31 requests in one minute (limit is 30)
    Then the 31st request is rejected with a rate limit error
    And the response includes retry-after information

  Scenario: Static analysis detects unreachable step
    Given the AI generates a workflow with step "notify_broker" that no other step transitions to
    And "notify_broker" is not the entry point
    When static analysis runs
    Then an error is reported: "unreachable step: notify_broker"
    And the diff cannot be applied until the error is fixed

  Scenario: Static analysis detects infinite loop
    Given the AI generates a workflow where step A → step B → step A with no exit condition
    When static analysis runs
    Then an error is reported: "infinite loop detected: A → B → A (no max retries or exit guard)"

  Scenario: Static analysis warns on dangling input
    Given the AI generates a step that reads from "case.data.applicant.employer_name"
    And no upstream step or channel populates that field
    When static analysis runs
    Then a warning is reported: "step 'employment_check' reads 'applicant.employer_name' which is not populated by any upstream step"
    And the diff can still be applied (warning, not error)

  Scenario: Static analysis warns on high-cost connector
    Given the AI generates a workflow using "experian" (cost.level = "very_high") for a basic company check
    And "companies-house" (cost.level = "free") could satisfy the requirement
    When static analysis runs
    Then a cost warning is reported: "step 'credit_check' uses Experian (very high cost) — consider Companies House for basic company data"

  Scenario: AI self-corrects after static analysis
    Given the AI generates YAML that passes schema validation
    But static analysis detects an unreachable step
    When the findings are fed back to the LLM
    Then the LLM produces corrected YAML with the unreachable step connected
    And the corrected YAML passes both schema validation and static analysis

  Scenario: Cost-aware generation prefers cheaper connectors
    Given the user asks "verify the company exists"
    And the plugin catalogue contains "companies-house" (free) and "experian" (very_high)
    When the assistant generates the workflow
    Then it uses "companies-house" for the verification step
    And includes a comment: "Using Companies House (free) for company verification"
```

---

## Spec 021 — Code Component

### Summary

A sandboxed JavaScript execution step powered by goja that handles bespoke workflow logic. The code step is a pressure valve — it lets users solve their own problems without requiring a custom plugin for every edge case.

### Dependencies

- Spec 002 (Execution Engine) — the code step is a step type.
- Spec 011 (Audit Trail) — code execution is audited.

### Data Model

```go
// CodeStepConfig is the configuration for a code step in workflow YAML
type CodeStepConfig struct {
    Script      string            `yaml:"script"`       // JavaScript source
    Timeout     int               `yaml:"timeout"`      // seconds, max 30
    MemoryLimit int               `yaml:"memory_limit"` // MB, max 64
    InputPaths  map[string]string `yaml:"input_paths"`  // name -> JSON path into case data
    OutputPath  string            `yaml:"output_path"`  // JSON path for result merge
}
```

```yaml
# Example in workflow YAML
- id: calculate_risk_band
  type: code
  config:
    timeout: 10
    input_paths:
      credit_score: case.data.applicant.credit_score
      loan_amount: case.data.loan.amount
      employment_years: case.data.applicant.employment_years
    output_path: case.data.computed.risk_band
    script: |
      const score = inputs.credit_score;
      const amount = inputs.loan_amount;
      const years = inputs.employment_years;

      let band = "high";
      if (score > 700 && amount < 50000 && years > 3) band = "low";
      else if (score > 600 && amount < 100000) band = "medium";

      return { risk_band: band, factors: { score, amount, years } };
```

### Behaviour

1. **goja runtime.** Each code step execution creates a fresh goja runtime. No state persists between invocations. The runtime is pre-configured with the curated stdlib.

2. **Curated stdlib available in scripts:**
   - `http.get(url, headers)`, `http.post(url, body, headers)`, `http.put(url, body, headers)` — routed through the Go-side controlled HTTP client.
   - `crypto.sha256(input)`, `crypto.hmac(key, input)`, `crypto.uuid()` — common crypto operations.
   - `base64.encode(input)`, `base64.decode(input)` — encoding utilities.
   - `date.now()`, `date.parse(str)`, `date.format(date, pattern)` — date handling.
   - `log.info(msg)`, `log.warn(msg)`, `log.error(msg)` — logging to Aceryx log stream.
   - `JSON.parse()`, `JSON.stringify()` — standard JSON (built into goja).

3. **HTTP sandboxing.** HTTP calls from scripts route through the same controlled client as plugin host functions: per-tenant domain allowlisting, private IP blocking, response size limits (default 5MB), request timeout (minimum of script-specified and system max).

4. **Not available in scripts:** `require()`, `import`, `eval()`, `Function()`, `setTimeout`, `setInterval`, `fetch`, filesystem access, process/OS access. Any attempt to access these throws a runtime error.

5. **Input injection.** The `input_paths` mapping resolves JSON paths against case data before script execution. The resolved values are available as `inputs.name` in the script.

6. **Output handling.** The script's return value is serialised to JSON and merged into case data at `output_path`. If the script returns `undefined` or `null`, no merge occurs. If the return value fails JSON serialisation, the step fails.

7. **Resource limits.** Execution time is bounded by `timeout` (default 10s, max 30s). Memory is bounded by `memory_limit` (default 32MB, max 64MB). Exceeding either limit terminates execution with a clear error.

8. **Audit.** The script source, inputs, outputs, execution time, and any errors are recorded in the audit trail. Script source is stored as-is (not hashed) because the audit must be able to replay "what logic ran."

### BDD Scenarios

```gherkin
Feature: Code Component

  Scenario: Execute a simple computation
    Given a code step with script "return { result: inputs.a + inputs.b }"
    And input_paths: a = case.data.x (value 5), b = case.data.y (value 3)
    When the step executes
    Then the output is {"result": 8}
    And the output is merged into case data at the configured output_path

  Scenario: HTTP call from script
    Given a code step that calls http.get("https://api.example.com/data")
    And the tenant allows domain "api.example.com"
    When the step executes
    Then the HTTP request is made through the controlled client
    And the response is available in the script

  Scenario: HTTP call to blocked domain
    Given a code step that calls http.get("https://blocked.example.com")
    And the tenant does not allow "blocked.example.com"
    When the step executes
    Then the HTTP call throws an error "domain not allowed: blocked.example.com"
    And the step fails

  Scenario: HTTP call to private IP blocked
    Given a code step that calls http.get("http://192.168.1.1/secret")
    When the step executes
    Then the HTTP call throws an error "request to private IP range blocked"

  Scenario: Timeout enforcement
    Given a code step with timeout 2 seconds and a script with an infinite loop
    When the step executes
    Then execution is terminated after 2 seconds
    And the step fails with error "execution timeout: 2s"

  Scenario: Disallowed operations
    Given a code step with script "require('fs')"
    When the step executes
    Then the script throws "require is not defined"
    And the step fails

  Scenario: Eval blocked
    Given a code step with script "eval('1+1')"
    When the step executes
    Then the script throws "eval is not allowed"

  Scenario: Null return produces no merge
    Given a code step with script "return null"
    When the step executes
    Then no data is merged into case data
    And the step succeeds

  Scenario: Script error recorded in audit
    Given a code step with a script that throws an error
    When the step executes and fails
    Then the audit record contains the script source, inputs, and the error message

  Scenario: Crypto stdlib available
    Given a code step with script "return { id: crypto.uuid(), hash: crypto.sha256('hello') }"
    When the step executes
    Then the output contains a valid UUID and a SHA-256 hex string

  Scenario: Fresh runtime per invocation
    Given a code step that sets a global variable "globalVar = 42"
    When the step executes twice on different cases
    Then each invocation starts with no globalVar defined
    And state does not leak between invocations
```

---

## Spec 022 — Channels & Data Ingestion

### Summary

Channels define how data enters Aceryx from external sources. Four core channel types are built in: email (IMAP), webhook (HTTP), form (public intake), and file drop (directory watch). Additional channel types are implemented as trigger plugins (spec 029). All channels feed into a unified processing pipeline.

### Dependencies

- Spec 001 (Postgres Schema) — channel configuration storage.
- Spec 007 (Vault) — attachment storage.
- Spec 003 (Case Management API) — case creation.
- Spec 024 (Plugin Runtime) — trigger plugin integration.

### Data Model

```sql
CREATE TABLE channels (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('email', 'webhook', 'form', 'file_drop', 'plugin')),
    plugin_id       TEXT,                  -- for type 'plugin', references the trigger plugin
    config          JSONB NOT NULL,        -- type-specific configuration
    case_type_id    UUID NOT NULL,         -- cases created by this channel
    workflow_id     UUID,                  -- workflow to trigger on case creation
    adapter_config  JSONB NOT NULL DEFAULT '{}', -- field mapping from inbound to case data
    dedup_config    JSONB NOT NULL DEFAULT '{}', -- deduplication rules
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE channel_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    channel_id      UUID NOT NULL REFERENCES channels(id),
    raw_payload     JSONB,                 -- original inbound data
    attachments     JSONB DEFAULT '[]',    -- [{vault_id, filename, content_type, size}]
    case_id         UUID,                  -- resulting case (null if deduped or failed)
    status          TEXT NOT NULL CHECK (status IN ('processed', 'deduped', 'failed')),
    error_message   TEXT,
    processing_ms   INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_channel_events_channel ON channel_events (channel_id, created_at DESC);
CREATE INDEX idx_channel_events_tenant ON channel_events (tenant_id, created_at DESC);
```

### Behaviour

1. **Unified pipeline.** Every channel type, including trigger plugins, feeds into the same pipeline: receive → dedup → adapt → store attachments → create/update case → trigger workflow → audit. The pipeline runs in a single database transaction.

2. **Email channel (IMAP).** Polls a configured IMAP mailbox at a configurable interval (default 60s). For each new email: extracts subject, body (plain and HTML), sender, recipients, and attachments. Attachments are stored in the Vault. The adapter maps email fields to case data fields.

3. **Webhook channel (HTTP).** Exposes an endpoint at `/api/v1/channels/webhook/:channel_id`. Supports configurable authentication: none, API key (header), HMAC signature verification, or Bearer token. The JSON/form body is parsed and fed into the adapter.

4. **Form channel.** Exposes a public form at `/intake/:channel_id` rendered from the case type's intake schema. Rate-limited per IP (default 10/minute). Optional CAPTCHA (hCaptcha). The form submission is the adapter input.

5. **File drop channel.** Watches a configured directory for new files. When a file appears: reads it, stores in Vault, extracts metadata (filename, size, content type), and feeds into the pipeline. The file is moved to a `processed/` subdirectory after successful ingestion.

6. **Plugin channels.** Channels of type `plugin` delegate to a trigger plugin (spec 029). The trigger plugin calls `host_create_case` when it has data, which routes through the same unified pipeline. Configuration is passed from the channel's `config` field to the trigger plugin.

7. **Deduplication.** Configurable dedup rules: by field hash (e.g. email message-id), by time window (ignore duplicates within N minutes), or by case match (if a case with matching field values already exists, update instead of create). Deduped events are recorded with status `deduped`.

8. **Adapter mapping.** The `adapter_config` defines how inbound fields map to case data. Supports direct field mapping, constant values, expressions (goja), and nested path resolution.

### BDD Scenarios

```gherkin
Feature: Channels & Data Ingestion

  Scenario: Email channel creates case from inbound email
    Given an email channel configured for mailbox "intake@lending.com"
    And the channel maps subject → case.data.reference, body → case.data.description
    When an email arrives with subject "App-12345" and a PDF attachment
    Then a case is created with data.reference = "App-12345"
    And the PDF is stored in the Vault
    And the case data contains an attachment reference with vault_id
    And a channel_event record is created with status "processed"

  Scenario: Webhook channel with HMAC verification
    Given a webhook channel with HMAC secret "webhook-secret"
    When a POST arrives at the webhook URL with a valid HMAC signature
    Then the payload is accepted and processed
    When a POST arrives with an invalid HMAC signature
    Then the request is rejected with 401
    And a channel_event is created with status "failed"

  Scenario: Form channel with rate limiting
    Given a form channel with rate limit 10/minute per IP
    When 11 submissions arrive from the same IP within 60 seconds
    Then the first 10 are processed
    And the 11th is rejected with 429 Too Many Requests

  Scenario: File drop channel
    Given a file drop channel watching /data/incoming
    When a file "application.pdf" appears in the directory
    Then the file is stored in the Vault
    And a case is created with attachment reference
    And the file is moved to /data/incoming/processed/

  Scenario: Deduplication by field
    Given an email channel with dedup on message-id
    When the same email (same message-id) is received twice
    Then only one case is created
    And the second event has status "deduped"

  Scenario: Plugin trigger channel
    Given a channel of type "plugin" referencing the "kafka-consumer" trigger plugin
    And the config specifies topic "loan-applications"
    When the trigger plugin receives a message on the topic
    And calls host_create_case with the parsed message data
    Then a case is created through the unified pipeline
    And a channel_event is recorded

  Scenario: Adapter field mapping with expression
    Given a webhook channel with adapter_config mapping:
      | Source | Target | Type |
      | payload.amount | case.data.loan.amount | direct |
      | | case.data.source | constant:"webhook" |
      | payload.first + " " + payload.last | case.data.applicant.full_name | expression |
    When a webhook payload arrives with {amount: 50000, first: "Alice", last: "Smith"}
    Then the case data has loan.amount = 50000, source = "webhook", applicant.full_name = "Alice Smith"

  Scenario: Pipeline atomicity
    Given a webhook channel
    When the case creation fails (e.g. invalid case type)
    Then no attachments are stored in the Vault
    And no channel_event with status "processed" is created
    And a channel_event with status "failed" is created with the error message

  Scenario: Channel management API
    Given a tenant with 3 configured channels
    When GET /api/v1/channels is called
    Then all 3 channels are returned with status, type, and event counts
    When a channel is disabled via PUT
    Then it stops processing inbound data
    And its enabled flag is false
```

---

## Spec 023 — Document Extraction

### Summary

AI-powered structured data extraction from documents with provenance-linked review UI. Every extracted field includes source coordinates for visual overlay. Human corrections are logged for future accuracy improvement.

### Dependencies

- Spec 007 (Vault) — documents are stored in the Vault.
- Spec 004 (Human Tasks) — extraction review is a human task.
- Spec 006 (Agent Steps) — extraction uses LLM invocation.
- Spec 009 (Schema-Driven Forms) — review form rendering.

### Data Model

```sql
CREATE TABLE extraction_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    case_id         UUID NOT NULL,
    document_id     UUID NOT NULL,          -- Vault reference
    schema_id       TEXT NOT NULL,           -- extraction schema identifier
    model_used      TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'review', 'accepted', 'rejected')),
    confidence      NUMERIC(4,3),           -- overall extraction confidence 0.000-1.000
    extracted_data  JSONB,                  -- structured extraction output
    raw_response    JSONB,                  -- raw LLM response (for debugging)
    processing_ms   INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE extraction_fields (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES extraction_jobs(id),
    field_name      TEXT NOT NULL,
    extracted_value TEXT,
    confidence      NUMERIC(4,3) NOT NULL,
    source_text     TEXT,                   -- exact text snippet from document
    page_number     INTEGER,
    bbox_x          NUMERIC(6,4),           -- bounding box, normalised 0.0-1.0
    bbox_y          NUMERIC(6,4),
    bbox_width      NUMERIC(6,4),
    bbox_height     NUMERIC(6,4),
    status          TEXT NOT NULL DEFAULT 'extracted' CHECK (status IN ('extracted', 'confirmed', 'corrected', 'rejected')),
    corrected_value TEXT                    -- set when human corrects
);

CREATE INDEX idx_extraction_fields_job ON extraction_fields (job_id);

CREATE TABLE extraction_corrections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    job_id          UUID NOT NULL REFERENCES extraction_jobs(id),
    field_name      TEXT NOT NULL,
    original_value  TEXT,
    corrected_value TEXT,
    confidence      NUMERIC(4,3),
    model_used      TEXT,
    corrected_by    UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Behaviour

1. **Extraction schemas.** An extraction schema defines what to extract: field names, types, descriptions, and validation rules. Schemas are registered per tenant and per case type. Example: "Loan Application PDF" schema extracts applicant_name, company_number, loan_amount, loan_purpose.

2. **Preprocessor registry.** Before LLM invocation, the document is preprocessed based on MIME type:
   - `application/pdf` → render each page to PNG images.
   - `image/*` → pass through.
   - `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` → parse to JSON.
   - `application/vnd.openxmlformats-officedocument.wordprocessingml.document` → extract text, or render to images if layout matters.
   - Unknown → reject with error.

3. **LLM invocation.** The preprocessed content and extraction schema are sent to a vision-capable LLM. The system prompt instructs the LLM to return JSON with field values, confidence per field, source text snippets, and bounding box coordinates.

4. **Provenance contract.** Every field in the LLM response must include `source_text` and bounding box coordinates (`page`, `x`, `y`, `width`, `height` normalised to 0.0–1.0). Fields without provenance are flagged with confidence 0.0 and automatically routed to human review.

5. **Confidence routing.** Overall confidence is the minimum of all field confidences. If overall confidence ≥ the configured threshold (default 0.85), the extraction is auto-accepted. Below threshold, a human review task is created.

6. **Review UI.** Side-by-side layout: document in left pane (PDF viewer with page navigation), extracted fields in right pane. Hovering on a field highlights the bounding box region in the document. Clicking a field scrolls and zooms to the source region. Each field has confirm/edit/reject actions.

7. **Correction feedback.** Every human correction (edit or rejection) creates an `extraction_corrections` record. This data supports future accuracy improvement: prompt tuning, confidence calibration, and few-shot example selection.

8. **Case data merge.** On acceptance (automatic or human-confirmed), extracted data is merged into case data at the configured path. The extraction job ID is recorded so provenance is traceable from case data back to the source document and specific regions.

### BDD Scenarios

```gherkin
Feature: Document Extraction

  Scenario: Extract data from a PDF with high confidence
    Given a loan application PDF in the Vault
    And an extraction schema for "Loan Application" with fields: applicant_name, company_number, loan_amount
    When the extraction step runs
    Then the LLM returns extracted values with confidence > 0.85 for all fields
    And each field includes source_text and bounding box coordinates
    And the extraction is auto-accepted
    And the extracted data is merged into case data

  Scenario: Low confidence triggers human review
    Given an extraction where the LLM returns company_number with confidence 0.6
    Then overall confidence is 0.6 (minimum of all fields)
    And a human review task is created
    And the task uses the side-by-side review UI

  Scenario: Review UI provenance overlay
    Given an extraction job with field "loan_amount" at page 2, bbox (0.3, 0.5, 0.15, 0.03)
    When the reviewer hovers over "loan_amount" in the right pane
    Then a highlight overlay appears in the document viewer at page 2, position (0.3, 0.5)
    When the reviewer clicks "loan_amount"
    Then the document scrolls to page 2 and zooms to the bounding box region

  Scenario: Reviewer corrects a field
    Given a field "applicant_name" extracted as "Alce Smith" with confidence 0.7
    When the reviewer edits it to "Alice Smith" and confirms
    Then the field status changes to "corrected"
    And an extraction_corrections record is created with original "Alce Smith" and corrected "Alice Smith"
    And the case data is updated with the corrected value

  Scenario: Reviewer rejects a field
    Given a field "company_number" extracted as "99999999"
    When the reviewer rejects the field with reason "not a valid company number"
    Then the field status changes to "rejected"
    And the extraction is marked for re-processing or manual entry

  Scenario: Field without provenance gets zero confidence
    Given the LLM returns a field value without source_text or bounding box
    Then the field confidence is set to 0.0
    And the extraction is routed to human review regardless of other field confidences

  Scenario: Spreadsheet preprocessing
    Given an uploaded Excel file (.xlsx) in the Vault
    When the extraction step runs
    Then the file is parsed to JSON (not rendered as images)
    And the LLM receives structured data instead of images
    And the extraction proceeds normally

  Scenario: Extraction audit trail
    Given an extraction job that was auto-accepted
    When an auditor queries the extraction history for the case
    Then the record includes: document_id, model_used, all field values with confidence, source locations, and processing time
    And the case data field traces back to the extraction job ID
```

---

# Integration Architecture

---

## Spec 027 — Core Drivers

### Summary

Core drivers are Go packages compiled into the Aceryx binary that provide connectivity to systems requiring native protocol implementations: databases, message queues, file transfer, and domain-specific protocols. Each driver implements a common interface for its category and is registered in the connector catalogue at startup.

### Dependencies

- Spec 001 (Postgres Schema) — driver configurations stored as part of workflow YAML.
- Spec 008 (RBAC) — permission checks for driver operations (e.g. db:write).
- Spec 011 (Audit Trail) — all driver operations are audited.

### Data Model

```go
// DBDriver is the interface for all database drivers
type DBDriver interface {
    ID() string
    Connect(ctx context.Context, config DBConfig) (*sql.DB, error)
    Ping(ctx context.Context, db *sql.DB) error
    Close(db *sql.DB) error
}

type DBConfig struct {
    Host        string `yaml:"host"`
    Port        int    `yaml:"port"`
    Database    string `yaml:"database"`
    User        string `yaml:"user"`
    Password    string `yaml:"password"`  // resolved from secret store
    SSLMode     string `yaml:"ssl_mode"`
    MaxConns    int    `yaml:"max_conns"`
    ReadOnly    bool   `yaml:"read_only"` // default true
    TimeoutSecs int    `yaml:"timeout"`   // query timeout
    RowLimit    int    `yaml:"row_limit"` // max rows returned, default 10000
}

// QueueDriver is the interface for all message queue drivers
type QueueDriver interface {
    ID() string
    Publish(ctx context.Context, config QueueConfig, topic string, message []byte) error
    Subscribe(ctx context.Context, config QueueConfig, topic string, handler MessageHandler) error
    Close() error
}

type MessageHandler func(message []byte, metadata map[string]string) error

// FileDriver is the interface for file transfer drivers
type FileDriver interface {
    ID() string
    List(ctx context.Context, config FileConfig, path string) ([]FileEntry, error)
    Read(ctx context.Context, config FileConfig, path string) (io.ReadCloser, error)
    Write(ctx context.Context, config FileConfig, path string, data io.Reader) error
    Delete(ctx context.Context, config FileConfig, path string) error
}

// ProtocolDriver is the interface for domain-specific protocol drivers
type ProtocolDriver interface {
    ID() string
    Parse(data []byte) (map[string]interface{}, error)
    Format(data map[string]interface{}) ([]byte, error)
}
```

### Behaviour

1. **Driver registration.** Each driver self-registers at init() time. The driver registry is a map of ID → driver instance. The registry is queried by the step executor to find the appropriate driver for a step's connector type.

2. **Connection pooling.** Database drivers maintain per-tenant connection pools. Pool configuration (max connections, idle timeout) is per-driver-instance. Pools are created on first use and destroyed when the tenant's configuration changes.

3. **Read-only by default.** Database query steps open read-only transactions by default. Write operations require explicit opt-in in the step configuration and the `connectors:db:write` RBAC permission.

4. **Query safety.** Parameterised queries only — no string interpolation. Query timeout enforced at the driver level. Row limit enforced by wrapping queries in a LIMIT clause (or equivalent). Results returned as `[]map[string]interface{}`.

5. **Queue drivers.** Publish is synchronous (returns when the message is acknowledged). Subscribe is used by trigger plugins — the driver provides the subscription loop, the trigger plugin provides the message handler.

6. **Secret resolution.** Connection strings and credentials reference the tenant's secret store. The driver layer resolves secret references before connecting. Plaintext credentials are never stored in workflow YAML.

7. **Licence gating.** Most drivers are available in the open-source tier. Financial protocols (SWIFT, FIX, ISO 8583) and healthcare protocols (HL7, FHIR) are gated by licence. The driver registry checks the licence before registering gated drivers.

### BDD Scenarios

```gherkin
Feature: Core Drivers

  Scenario: Database query step — read-only
    Given a workflow step configured to query PostgreSQL with "SELECT * FROM orders WHERE status = $1"
    And parameter $1 = "pending"
    When the step executes
    Then the query runs in a read-only transaction
    And results are returned as a JSON array of objects
    And the result is merged into case data

  Scenario: Database query — row limit
    Given a query that would return 50,000 rows and row_limit = 10000
    When the step executes
    Then only 10,000 rows are returned
    And a warning is logged: "query result truncated at 10000 rows"

  Scenario: Database write requires permission
    Given a workflow step configured to INSERT into a MySQL table
    And the step config has read_only = false
    And the executing user does not have connectors:db:write permission
    When the step executes
    Then it fails with "insufficient permissions: connectors:db:write required"

  Scenario: Queue publish
    Given a workflow step configured to publish to Kafka topic "events"
    When the step executes with message payload from case data
    Then the message is published to the topic
    And the step waits for broker acknowledgement before completing

  Scenario: SFTP file read
    Given a workflow step configured to read "/incoming/report.csv" via SFTP
    When the step executes
    Then the file is downloaded via the SFTP driver
    And the content is stored in the Vault
    And a vault reference is added to case data

  Scenario: Secret resolution for credentials
    Given a database step with password referencing secret "db-password"
    And the tenant's secret store contains "db-password" = "s3cret"
    When the step executes
    Then the driver connects using password "s3cret"
    And the workflow YAML does not contain the plaintext password

  Scenario: Connection pool reuse
    Given two workflow steps in the same case both query the same PostgreSQL instance
    When both steps execute
    Then both use the same connection pool
    And the pool connection count does not exceed max_conns

  Scenario: SWIFT message parsing
    Given a workflow step configured to parse a SWIFT MT103 message
    And the licence includes financial protocol support
    When the step receives a raw MT103 message
    Then the driver parses it into structured fields: sender, receiver, amount, currency, reference
    And the parsed data is merged into case data
```

---

## Spec 028 — HTTP Connector Framework

### Summary

The HTTP connector framework defines the pattern for WASM step plugins that interact with external systems over HTTP. It provides shared infrastructure for authentication, request building, response parsing, and error handling that all HTTP-based plugins leverage through host functions.

### Dependencies

- Spec 024 (Plugin Runtime) — plugins execute in the WASM runtime.
- Spec 025 (Plugin SDK) — plugins use the SDK to call host functions.
- Spec 026 (Plugin Manifest) — plugins declare their configuration.

### Behaviour

1. **Authentication patterns.** The `host_http_request` host function supports common auth patterns injected by the host based on the plugin's configured credentials:
   - **API Key** — injected as a header (e.g. `X-API-Key`, `Authorization: ApiKey ...`).
   - **Bearer Token** — injected as `Authorization: Bearer ...`.
   - **Basic Auth** — injected as `Authorization: Basic base64(user:pass)`.
   - **OAuth2 Client Credentials** — the host manages token acquisition, caching, and refresh. The plugin receives a valid token transparently.
   - **HMAC Signature** — the host computes the signature based on the plugin's signing configuration.

   The plugin never handles raw credentials — it calls `host_http_request` and the host injects auth.

2. **Request building.** The plugin constructs the request URL, method, headers, and body. URL parameters can be templated from case data. The host enforces URL validation (HTTPS required for production, HTTP allowed for localhost in development mode).

3. **Response handling.** The host returns status code, headers, and body. The plugin is responsible for parsing the response and extracting relevant data. The SDK provides helpers: `resp.JSON()`, `resp.Text()`, `resp.XML()`.

4. **Error handling conventions.** HTTP connectors should:
   - Return `sdk.Error` for non-recoverable errors (auth failure, invalid request).
   - Return the HTTP status and body for downstream handling on expected error responses (e.g. "no results found" from a search API).
   - Log warnings for retryable errors when the host retry policy will handle them.

5. **Rate limiting.** The host tracks request rates per plugin per external domain. If a plugin's manifest declares rate limits, the host enforces them by delaying requests. Exceeding rate limits returns a retryable error.

6. **Pagination.** For APIs that paginate, the plugin handles pagination logic internally — looping through pages and accumulating results. The SDK provides a `PaginatedRequest` helper that manages cursor/offset tracking.

### BDD Scenarios

```gherkin
Feature: HTTP Connector Framework

  Scenario: API key authentication
    Given a plugin configured with auth type "api_key" and header "X-API-Key"
    When the plugin calls host_http_request
    Then the host injects the X-API-Key header with the stored secret value
    And the plugin does not see the raw API key

  Scenario: OAuth2 client credentials flow
    Given a plugin configured with auth type "oauth2_client_credentials"
    And the OAuth2 token endpoint, client_id, and client_secret are configured
    When the plugin makes its first request
    Then the host acquires an access token from the token endpoint
    And injects it as a Bearer token in the request
    When the plugin makes a second request within the token's lifetime
    Then the cached token is reused without another token request

  Scenario: OAuth2 token refresh
    Given a cached OAuth2 token that has expired
    When the plugin makes a request
    Then the host automatically refreshes the token
    And the request proceeds with the new token

  Scenario: HTTPS enforcement
    Given a plugin that calls host_http_request with an HTTP URL in production mode
    Then the request is rejected with "HTTPS required in production mode"

  Scenario: Rate limiting enforcement
    Given a plugin manifest declares rate_limit: 10 requests/second for api.example.com
    When the plugin makes 15 requests in 1 second
    Then the first 10 proceed immediately
    And requests 11-15 are delayed to stay within the rate limit
    And all 15 eventually complete

  Scenario: Paginated API call
    Given a Companies House API that returns 20 results per page
    And the total result set is 50 items
    When the plugin uses the SDK's PaginatedRequest helper
    Then 3 API calls are made (pages 1, 2, 3)
    And the accumulated result contains 50 items

  Scenario: Error propagation
    Given a plugin that calls an API returning 403 Forbidden
    When the response is received
    Then the plugin can inspect resp.Status (403) and resp.Body
    And the plugin decides whether to return sdk.Error or handle gracefully
```

---

## Spec 029 — Trigger Plugin Framework

### Summary

Trigger plugins are long-running WASM modules that listen for external events and push data into Aceryx. They handle queue consumption, file watching, scheduled polling, and CDC listening. This spec defines the lifecycle, host function interface, and reliability guarantees for trigger plugins.

### Dependencies

- Spec 024 (Plugin Runtime) — trigger lifecycle management.
- Spec 022 (Channels) — triggers feed into the channel pipeline.
- Spec 027 (Core Drivers) — triggers use core drivers for protocol connectivity.

### Data Model

```sql
CREATE TABLE trigger_instances (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    channel_id      UUID NOT NULL REFERENCES channels(id),
    plugin_id       TEXT NOT NULL,
    plugin_version  TEXT NOT NULL,              -- pinned at start time
    status          TEXT NOT NULL CHECK (status IN ('starting', 'running', 'stopping', 'stopped', 'error')),
    started_at      TIMESTAMPTZ,
    stopped_at      TIMESTAMPTZ,
    events_received BIGINT NOT NULL DEFAULT 0,
    events_processed BIGINT NOT NULL DEFAULT 0,
    events_failed   BIGINT NOT NULL DEFAULT 0,
    last_event_at   TIMESTAMPTZ,
    error_message   TEXT,
    config          JSONB NOT NULL
);

CREATE INDEX idx_trigger_instances_channel ON trigger_instances (channel_id);

-- Checkpoint persistence for host-managed trigger state
CREATE TABLE trigger_checkpoints (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trigger_id      UUID NOT NULL REFERENCES trigger_instances(id),
    checkpoint_key  TEXT NOT NULL,          -- e.g. "kafka:topic:partition" or "sftp:/path"
    checkpoint_value TEXT NOT NULL,         -- e.g. offset number, file modification time
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(trigger_id, checkpoint_key)
);
```

### Behaviour

1. **Trigger lifecycle.** Start → Running → Stop. The runtime calls the plugin's `Start` export in a goroutine. The plugin runs its event loop until the runtime calls `Stop`. Graceful shutdown allows up to 30 seconds for in-flight work to complete.

2. **Trigger contract enforcement.** Every trigger plugin must declare a `trigger_contract` in its manifest (spec 026). The runtime uses this contract to configure behaviour at startup. Triggers without a valid contract fail to start.

3. **Delivery semantics.** Governed by the trigger contract's `delivery` field:
   - **at_least_once** (recommended for most triggers). The host defers message acknowledgement until the unified pipeline confirms successful processing. If the pipeline fails, the message is not acknowledged and the source system (broker, poller) will redeliver. Duplicates are possible and must be handled by the pipeline's dedup layer (spec 022).
   - **exactly_once.** True exactly-once delivery requires the source's acknowledgement mechanism to participate in the same atomicity boundary as case creation. The host wraps acknowledgement and case creation in a single database transaction only when this is genuinely achievable — specifically, when the source is itself Postgres-backed (e.g. Postgres logical replication, PGMQ) so that the source ack and the case INSERT share the same transaction. For external brokers like Kafka or RabbitMQ, exactly-once is not supported because the broker's ack protocol cannot participate in a Postgres transaction — declaring `exactly_once` with an incompatible driver fails at startup with an explicit error. This prevents implementations that claim exactly-once but silently deliver at-least-once with dedup heuristics. If you need Kafka with strong guarantees, use `at_least_once` with the pipeline dedup layer (spec 022).
   - **best_effort.** The host acknowledges immediately on receipt, before pipeline processing. Fastest, but messages may be lost if the pipeline fails. Appropriate for non-critical telemetry or idempotent retry sources.

4. **Concurrency model.** Governed by the trigger contract's `concurrency` field:
   - **single.** One goroutine processes one message at a time. Ordering is preserved. Throughput is limited by pipeline latency.
   - **parallel.** A configurable worker pool (default 4 workers) processes messages concurrently. Ordering is not guaranteed. Throughput scales with worker count. Workers share the same WASM compiled module but have isolated instances.

5. **State and checkpointing.** Governed by the trigger contract's `state` and `checkpoint` fields:
   - **host_managed.** The runtime persists consumer offsets, polling cursors, or file positions after each successful acknowledgement (or periodically, or on shutdown, per the `checkpoint.strategy`). On restart, the trigger resumes from the last persisted checkpoint. The runtime stores checkpoints in a `trigger_checkpoints` table.
   - **plugin_managed.** The plugin manages its own state via `host_case_set` or a custom mechanism. The runtime does not persist checkpoints. The plugin is responsible for resume-from-where-I-left-off logic.

6. **Driver-mediated I/O.** Trigger plugins that consume from queues or watch files do so through host functions that wrap core drivers:
   - `host_queue_consume(driver_id, config, topic)` — blocks until a message is available, returns message bytes, metadata, and a message_id for acknowledgement.
   - `host_queue_ack(driver_id, message_id)` — explicitly acknowledges a message. For `at_least_once` delivery, this is called by the host after pipeline success, not by the plugin directly. For `best_effort`, this is called immediately.
   - `host_file_watch(driver_id, config, path)` — blocks until a file change is detected, returns file path and event type.
   - `host_poll_http` is NOT a host function. `PollHTTP` in the SDK is sugar over `host_http_request` + sleep, implemented entirely in the SDK library. Audit logging and rate limiting apply through the underlying HTTP host function. This keeps the host function surface minimal while providing a convenient polling pattern for trigger authors.

7. **Event processing.** When a trigger plugin has data, it calls `host_create_case(case_type, data)` to create a case through the unified channel pipeline, or `host_emit_event(type, payload)` for non-case events (e.g. updating an existing case).

8. **Backpressure.** When the pipeline is slow (case creation taking >1s average), the runtime applies backpressure to the trigger:
   - For `single` concurrency: the plugin naturally blocks on `host_create_case`.
   - For `parallel` concurrency: the worker pool's input channel has a bounded buffer (default 100). When the buffer is full, `host_queue_consume` blocks until a slot opens. This prevents unbounded memory growth from fast producers.

9. **Health monitoring.** The runtime periodically checks trigger goroutines. If a trigger goroutine exits unexpectedly (WASM trap, panic, unhandled error), the status is set to `error`, the error is logged, and the trigger is automatically restarted after a backoff delay (1s, 2s, 4s, 8s, max 60s). On restart with `host_managed` state, the trigger resumes from the last checkpoint.

10. **Metrics.** The `trigger_instances` table tracks events received, processed, and failed. These counters are updated atomically and visible in the admin UI. Additional metrics: current consumer lag (for queue-based triggers), last checkpoint offset, and worker pool utilisation (for parallel triggers).

### BDD Scenarios

```gherkin
Feature: Trigger Plugin Framework

  Scenario: Kafka trigger consumes messages and creates cases
    Given a trigger plugin "kafka-consumer" configured for topic "applications"
    And the plugin calls host_queue_consume in a loop
    When a message arrives on the Kafka topic
    Then host_queue_consume returns the message
    And the plugin parses it and calls host_create_case
    And a case is created through the unified pipeline
    And the Kafka message is acknowledged

  Scenario: Failed pipeline does not acknowledge message
    Given a trigger consuming from a queue
    When host_create_case fails (e.g. invalid case type)
    Then the queue message is not acknowledged
    And the message will be redelivered by the broker
    And events_failed is incremented

  Scenario: Trigger automatic restart on crash
    Given a running trigger plugin that encounters a WASM trap
    Then the trigger status changes to "error"
    And after a 1-second backoff, the trigger is automatically restarted
    And the restart is logged

  Scenario: Trigger backoff escalation
    Given a trigger that fails 4 times consecutively
    Then restart delays are: 1s, 2s, 4s, 8s
    When the trigger runs successfully after the 5th restart
    Then the backoff resets to 1s

  Scenario: Graceful shutdown
    Given a running trigger plugin processing a message
    When the runtime sends Stop
    Then the plugin completes the in-flight message processing
    And calls host_create_case for the final message
    And exits within 30 seconds
    And the trigger status changes to "stopped"

  Scenario: Forced shutdown on timeout
    Given a trigger plugin that does not exit within 30 seconds of Stop
    Then the goroutine is terminated
    And the trigger status changes to "stopped" with a warning

  Scenario: File watch trigger
    Given a trigger plugin "sftp-watcher" configured to watch /incoming on an SFTP server
    When a new file appears
    Then host_file_watch returns the file path
    And the plugin reads the file via host functions
    And creates a case with the file content

  Scenario: Trigger metrics visible in admin
    Given a trigger that has processed 1000 events, 5 failed
    When GET /api/v1/admin/triggers/:id is called
    Then the response includes events_received: 1005, events_processed: 1000, events_failed: 5
    And the response includes last_checkpoint_offset and consumer_lag

  Scenario: At-least-once delivery — ack deferred until pipeline success
    Given a trigger with trigger_contract.delivery = "at_least_once"
    When a message is consumed and host_create_case succeeds
    Then the message is acknowledged after case creation commits
    When a message is consumed and host_create_case fails
    Then the message is NOT acknowledged
    And the broker will redeliver the message

  Scenario: Best-effort delivery — immediate ack
    Given a trigger with trigger_contract.delivery = "best_effort"
    When a message is consumed
    Then the message is acknowledged immediately
    And host_create_case runs asynchronously
    And if pipeline fails, the message is lost (acceptable for this delivery mode)

  Scenario: Exactly-once delivery — transactional ack
    Given a trigger with trigger_contract.delivery = "exactly_once"
    And the source supports transactional acknowledgement
    When a message is consumed
    Then acknowledgement and case creation run in a single database transaction
    And if either fails, both roll back
    And no duplicate or lost messages occur

  Scenario: Exactly-once delivery — incompatible source rejected
    Given a trigger with trigger_contract.delivery = "exactly_once"
    And the source driver does not support transactional acknowledgement
    When the trigger starts
    Then it fails with error "exactly_once delivery not supported for driver: kafka (use at_least_once)"

  Scenario: Parallel concurrency — worker pool
    Given a trigger with trigger_contract.concurrency = "parallel" and 4 workers
    When 10 messages arrive rapidly
    Then up to 4 messages are processed concurrently
    And message ordering is NOT guaranteed
    And all 10 messages are eventually processed

  Scenario: Single concurrency — ordered processing
    Given a trigger with trigger_contract.concurrency = "single"
    When 5 messages arrive
    Then they are processed one at a time in order
    And message 2 does not start until message 1 completes

  Scenario: Backpressure — buffer full
    Given a parallel trigger with buffer size 100
    And the pipeline is slow (1s per case creation)
    When 150 messages arrive in 1 second
    Then the first 100 fill the buffer
    And host_queue_consume blocks for messages 101-150 until buffer slots open
    And no out-of-memory condition occurs

  Scenario: Host-managed checkpointing — resume on restart
    Given a trigger with trigger_contract.state = "host_managed"
    And checkpoint.strategy = "per_message"
    When the trigger processes 100 messages and the checkpoint is at offset 100
    And the trigger crashes and restarts
    Then the trigger resumes from offset 100
    And messages 1-100 are not reprocessed

  Scenario: Host-managed checkpointing — periodic
    Given a trigger with trigger_contract.state = "host_managed"
    And checkpoint.strategy = "periodic" with interval_ms = 5000
    When the trigger processes 50 messages over 3 seconds then crashes
    Then the last checkpoint was written at the most recent 5s boundary
    And on restart, some messages since the last checkpoint may be reprocessed (at-least-once)

  Scenario: Plugin-managed state
    Given a trigger with trigger_contract.state = "plugin_managed"
    When the trigger restarts
    Then the runtime does NOT restore any checkpoint
    And the plugin is responsible for its own resume logic
```

---

# AI & Knowledge

---

## Spec 030 — AI Component Registry

### Summary

The AI Component Registry is a YAML-driven catalogue of prompt-wrapper components. Each component is a combination of a system prompt, an output schema, and model hints that appears as a distinct step type in the builder sidebar. The registry turns prompt engineering into a product feature.

### Dependencies

- Spec 006 (Agent Steps) — components use the agent step execution path.
- Spec 031 (LLM Adapter Framework) — components call LLMs through the adapter.
- Spec 026 (Plugin Manifest) — components follow the manifest pattern.

### Data Model

```go
type AIComponentDef struct {
    ID             string         `yaml:"id"`
    DisplayLabel   string         `yaml:"display_label"`
    Category       string         `yaml:"category"`
    Description    string         `yaml:"description"`
    Icon           string         `yaml:"icon"`
    Tier           string         `yaml:"tier"` // "open_source" or "commercial"
    InputSchema    JSONSchema     `yaml:"input_schema"`
    OutputSchema   JSONSchema     `yaml:"output_schema"`
    SystemPrompt   string         `yaml:"system_prompt"`
    UserPromptTmpl string         `yaml:"user_prompt_template"`
    ModelHints     ModelHints     `yaml:"model_hints"`
    ConfigFields   []ConfigField  `yaml:"config_fields"`
    Confidence     ConfidenceConfig `yaml:"confidence"`
}

type ModelHints struct {
    RequiresVision bool   `yaml:"requires_vision"`
    PreferredSize  string `yaml:"preferred_size"` // "small", "medium", "large"
    MaxTokens      int    `yaml:"max_tokens"`
}

type ConfidenceConfig struct {
    FieldPath       string  `yaml:"field_path"`      // path to confidence in output
    AutoAcceptAbove float64 `yaml:"auto_accept_above"` // threshold for auto-acceptance
    EscalateBelow   float64 `yaml:"escalate_below"`    // threshold for human escalation
}
```

### Behaviour

1. **Loading.** The registry scans a configured directory (default `/etc/aceryx/ai-components/`) for YAML files. Each file defines one AI component. Files are validated against the schema on load. Invalid files are skipped with warnings.

2. **Execution path.** All AI components share one execution path: resolve inputs from case data → assemble prompt from templates → call LLM (via adapter, spec 031) → validate output against schema → apply confidence routing → merge into case data. No per-component Go code exists.

3. **Prompt templating.** `user_prompt_template` supports Go template syntax with access to the input fields: `{{.Input.fieldname}}`. The system prompt is sent as-is. Config fields (user-configured values from the builder) are available as `{{.Config.fieldname}}`.

4. **Output validation.** The LLM response is parsed as JSON and validated against `output_schema`. If validation fails, the LLM is re-prompted with the validation error (up to 2 retries). If all retries fail, the step fails with the validation errors.

5. **Confidence routing.** If the output includes a confidence field (declared in `confidence.field_path`), the engine routes based on thresholds: above `auto_accept_above` → auto-merge, below `escalate_below` → create human review task, between → merge with warning flag.

6. **Custom components.** Tenants can add their own AI component YAML files via the admin UI or API. These are stored per-tenant and supplement the global registry. This enables customer-specific prompt components without code changes.

### BDD Scenarios

```gherkin
Feature: AI Component Registry

  Scenario: Load components from directory
    Given 5 YAML files in the AI components directory
    When the registry loads
    Then 5 AI components appear in the builder sidebar under their categories

  Scenario: Execute sentiment analysis component
    Given the "sentiment_analysis" component is configured on a step
    And the step reads from case.data.customer_feedback
    When the step executes
    Then the input text is assembled into the prompt using the template
    And the LLM is called with the system prompt and user prompt
    And the response is validated against the output schema
    And the result (sentiment, score, confidence) is merged into case data

  Scenario: Output validation failure with retry
    Given the LLM returns invalid JSON on the first attempt
    When the output fails schema validation
    Then the LLM is re-prompted with the validation error
    And the second attempt returns valid output
    And the valid output is merged into case data

  Scenario: Confidence-based routing — auto-accept
    Given a component with auto_accept_above = 0.9
    And the LLM returns confidence = 0.95
    Then the output is auto-merged into case data without human review

  Scenario: Confidence-based routing — escalate
    Given a component with escalate_below = 0.7
    And the LLM returns confidence = 0.5
    Then a human review task is created
    And the extracted data is presented for confirmation

  Scenario: Tenant adds custom component
    Given a tenant uploads a YAML AI component "credit_risk_assessment"
    When the registry reloads
    Then "Credit Risk Assessment" appears in the builder for that tenant only
    And other tenants do not see it

  Scenario: Invalid YAML skipped
    Given a YAML file with missing "system_prompt" field
    When the registry loads
    Then the file is skipped with a warning
    And all other components load successfully
```

---

## Spec 031 — LLM Adapter Framework

### Summary

The LLM adapter framework provides a uniform interface for invoking language models from multiple providers. It handles model routing, token counting, rate limiting, cost tracking, and failover. Both the AI assistant (spec 020) and AI components (spec 030) use this framework.

### Dependencies

- Spec 008 (RBAC) — tenant-level LLM configuration.
- Spec 011 (Audit Trail) — LLM invocations are audited.

### Data Model

```sql
CREATE TABLE llm_provider_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    provider        TEXT NOT NULL CHECK (provider IN ('openai', 'anthropic', 'google', 'cohere', 'mistral', 'ollama', 'custom')),
    name            TEXT NOT NULL,         -- display name
    endpoint_url    TEXT,                  -- custom/ollama endpoint
    api_key_secret  TEXT NOT NULL,         -- reference to secret store
    default_model   TEXT NOT NULL,
    max_tokens      INTEGER DEFAULT 4096,
    temperature     NUMERIC(3,2) DEFAULT 0.7,
    is_default      BOOLEAN DEFAULT FALSE, -- tenant's default provider
    enabled         BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE llm_invocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    provider_id     UUID NOT NULL REFERENCES llm_provider_configs(id),
    model           TEXT NOT NULL,
    purpose         TEXT NOT NULL,          -- 'assistant', 'ai_component', 'extraction', 'embedding'
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    total_tokens    INTEGER,
    duration_ms     INTEGER NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('success', 'error', 'rate_limited')),
    error_message   TEXT,
    cost_usd        NUMERIC(10,6),         -- estimated cost
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_llm_invocations_tenant ON llm_invocations (tenant_id, created_at DESC);
```

```go
type LLMAdapter interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    SupportsVision() bool
    ModelInfo() ModelInfo
}

type ChatRequest struct {
    SystemPrompt string
    Messages     []Message
    Model        string       // override default
    MaxTokens    int
    Temperature  float64
    JSONMode     bool         // force JSON response
    Images       []Image      // for vision models
}

type ChatResponse struct {
    Content      string
    InputTokens  int
    OutputTokens int
    Model        string
    FinishReason string
}
```

### Behaviour

1. **Provider abstraction.** Each provider (OpenAI, Anthropic, Google, Cohere, Mistral, Ollama, custom OpenAI-compatible) implements the `LLMAdapter` interface. The adapter handles provider-specific API formatting, error mapping, and response parsing.

2. **Model routing.** When a component requests a model by hint (e.g. `preferred_size: small`), the framework maps the hint to the tenant's configured model. Hints are advisory — the tenant controls which actual model runs.

3. **JSON mode.** When `JSONMode` is true, the adapter uses provider-specific JSON mode features (OpenAI's response_format, Anthropic's tool use, etc.) to maximise the chance of valid JSON output.

4. **Token tracking.** Every invocation records input/output tokens and estimated cost. Tenants can set monthly token budgets with alerts.

5. **Rate limiting.** Per-provider rate limits are enforced at the adapter level. Requests exceeding limits are queued with backoff. Persistent rate limiting returns an error to the caller.

6. **Failover.** If a tenant configures multiple providers, the framework supports failover: if the primary provider returns an error or times out, the request is retried on the secondary provider.

7. **Local model support.** The Ollama and custom adapters point to local endpoints. No data leaves the customer's network. This is critical for air-gapped deployments.

### BDD Scenarios

```gherkin
Feature: LLM Adapter Framework

  Scenario: Chat completion via OpenAI
    Given a tenant configured with OpenAI as default provider
    When an AI component requests a chat completion
    Then the adapter formats the request for the OpenAI API
    And the response is parsed into a ChatResponse
    And an llm_invocations record is created with token counts

  Scenario: JSON mode enforcement
    Given a chat request with JSONMode = true
    When sent to the OpenAI adapter
    Then the request includes response_format: {type: "json_object"}
    When sent to the Anthropic adapter
    Then the system prompt is augmented with JSON output instructions

  Scenario: Provider failover
    Given tenant configured with primary = OpenAI, secondary = Anthropic
    When the OpenAI API returns a 500 error
    Then the request is retried on the Anthropic adapter
    And the response is returned from Anthropic
    And the invocation log records both attempts

  Scenario: Local model via Ollama
    Given a tenant configured with Ollama at http://localhost:11434
    When an AI component requests a chat completion
    Then the request is sent to the local Ollama endpoint
    And no data leaves the network

  Scenario: Token budget enforcement
    Given a tenant with a monthly budget of 1,000,000 tokens
    And 999,500 tokens have been used this month
    When a request estimated at 1,000 tokens is made
    Then the request proceeds (within budget)
    When the next request would exceed the budget
    Then an alert is generated
    And the request is still processed (budgets are alerts, not hard limits)

  Scenario: Rate limiting with backoff
    Given the OpenAI adapter encounters a 429 rate limit response
    Then the request is retried after the Retry-After delay
    And subsequent requests are throttled to avoid further rate limiting
```

---

## Spec 032 — RAG Infrastructure

### Summary

Retrieval-Augmented Generation infrastructure: document chunking, embedding generation, vector storage in pgvector, and hybrid search (vector + full-text with Reciprocal Rank Fusion). Provides knowledge bases that agent steps and AI components can query for contextual information.

### Dependencies

- Spec 007 (Vault) — source documents.
- Spec 031 (LLM Adapter) — embedding generation.
- Spec 006 (Agent Steps) — context assembly uses vector search.

### Data Model

```sql
CREATE TABLE knowledge_bases (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            TEXT NOT NULL,
    description     TEXT,
    chunking_strategy TEXT NOT NULL DEFAULT 'recursive' CHECK (chunking_strategy IN ('fixed', 'semantic', 'recursive', 'sliding')),
    chunk_size      INTEGER NOT NULL DEFAULT 512,     -- tokens
    chunk_overlap   INTEGER NOT NULL DEFAULT 50,      -- tokens
    embedding_model TEXT NOT NULL DEFAULT 'text-embedding-3-small',
    embedding_dims  INTEGER NOT NULL DEFAULT 1536,
    document_count  INTEGER NOT NULL DEFAULT 0,
    chunk_count     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE knowledge_documents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id),
    vault_document_id UUID NOT NULL,          -- reference to Vault
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'chunking', 'embedding', 'ready', 'error')),
    chunk_count     INTEGER DEFAULT 0,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE document_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    knowledge_base_id UUID NOT NULL REFERENCES knowledge_bases(id),
    document_id     UUID NOT NULL REFERENCES knowledge_documents(id),
    content         TEXT NOT NULL,
    token_count     INTEGER NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    embedding       vector(1536),
    content_tsv     tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_chunks_embedding ON document_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_chunks_fts ON document_chunks USING gin(content_tsv);
CREATE INDEX idx_chunks_kb ON document_chunks (knowledge_base_id);
CREATE INDEX idx_chunks_doc ON document_chunks (document_id);
```

### Behaviour

1. **Document ingestion.** When a document is added to a knowledge base, it enters a background processing pipeline: fetch from Vault → preprocess (extract text from PDF/DOCX/etc.) → chunk → embed → store.

2. **Chunking strategies.** As defined in the knowledge base configuration. The chunker produces chunks with metadata: source document ID, page number, section title, character offsets, chunk index.

3. **Embedding generation.** Chunks are embedded in batches using the configured embedding provider (via spec 031). Rate limiting and retry are handled by the LLM adapter. Embeddings are stored as pgvector columns.

4. **Vector search.** `SearchVector(query_embedding, top_k, min_score, filters)` — returns chunks ordered by cosine similarity. Tenant isolation is enforced via the `tenant_id` filter.

5. **Full-text search.** `SearchFullText(query_text, top_k, filters)` — uses PostgreSQL `ts_query` against the generated `tsvector` column.

6. **Hybrid search (default).** Combines vector and full-text results using Reciprocal Rank Fusion (RRF): for each result, `rrf_score = Σ 1/(k + rank_i)` where k=60 (standard constant) and rank_i is the result's rank in each search method. Results are sorted by RRF score.

7. **Context assembly.** Agent steps (spec 006) can include `vector_search` context sources. The search query is templated from case data. Retrieved chunks are injected into the LLM prompt with their provenance metadata, enabling the LLM to cite sources.

8. **Re-indexing.** When the chunking strategy or embedding model changes, the knowledge base can be re-indexed: all chunks are deleted and regenerated from source documents.

### BDD Scenarios

```gherkin
Feature: RAG Infrastructure

  Scenario: Upload document to knowledge base
    Given a knowledge base "Lending Policies"
    When a PDF document "risk-framework.pdf" is uploaded
    Then the document status becomes "chunking"
    And the PDF is extracted to text
    And text is split into chunks using the recursive strategy
    And chunks are embedded using the configured model
    And chunk records are created with embeddings
    And the document status becomes "ready"

  Scenario: Vector search returns relevant chunks
    Given a knowledge base with 100 chunks from 5 documents
    When a vector search is performed with query "maximum loan amount for first-time buyers"
    Then the top-k results are returned ordered by cosine similarity
    And each result includes the chunk content, score, and source document reference

  Scenario: Full-text search
    Given chunks containing the phrase "Section 4.2.1 risk appetite"
    When a full-text search is performed with query "Section 4.2.1"
    Then chunks containing that phrase are returned
    And results are ranked by PostgreSQL ts_rank

  Scenario: Hybrid search with RRF
    Given a query that matches some chunks semantically and others by keyword
    When hybrid search is performed
    Then results from both vector and full-text search are combined
    And RRF scoring produces a merged ranking
    And the top result is the most relevant across both methods

  Scenario: Agent step uses RAG context
    Given a workflow step with context_source type "vector_search"
    And query_template "lending policy for {{case.data.loan.purpose}} loans"
    When the step executes with case.data.loan.purpose = "property"
    Then the query "lending policy for property loans" is embedded
    And top-5 relevant chunks are retrieved
    And the chunks are included in the LLM prompt context

  Scenario: Tenant isolation
    Given tenant A and tenant B each have a knowledge base
    When tenant A performs a search
    Then only tenant A's chunks are returned
    And tenant B's chunks are never visible

  Scenario: Re-index knowledge base
    Given a knowledge base with 50 chunks using "fixed" chunking strategy
    When the strategy is changed to "semantic" and re-index is triggered
    Then all existing chunks are deleted
    And source documents are re-chunked with the new strategy
    And new embeddings are generated
    And the knowledge base is updated with new chunk counts

  Scenario: Large document batch embedding
    Given a document that produces 200 chunks
    When embedding is performed
    Then chunks are embedded in batches (e.g. 50 at a time)
    And rate limiting is respected between batches
    And all 200 chunks are stored with embeddings
```

---

# Performance & Packaging

---

## Spec 033 — Lightweight Execution Mode

### Summary

An optional per-workflow execution mode that prioritises throughput over durability. Workflows run in-memory with batch audit writes on completion, eliminating per-step database round-trips. Designed for high-volume integration flows (webhooks, queue processing) where individual execution durability is not required.

### Dependencies

- Spec 002 (Execution Engine) — extends the engine with an alternative execution path.
- Spec 011 (Audit Trail) — batch audit writing.

### Behaviour

1. **Opt-in per workflow.** `execution_mode: lightweight` in the workflow YAML. Default remains `standard`.

2. **In-memory execution.** Step transitions are tracked in memory only. No per-step database writes. The full execution trace (steps, inputs, outputs, timings) is held in a struct until completion.

3. **Batch audit on completion.** On workflow completion (success or failure), the entire execution trace is written to the audit trail in a single database transaction: one workflow event, one record per step, all in one INSERT batch.

4. **Constraints.** Lightweight mode rejects workflows containing: human task steps, SLA timers, agent steps with human escalation enabled, or any step type that requires durable state between engine restarts. Validation happens at workflow publish time, not at runtime.

5. **No recovery.** If the process crashes mid-workflow, the in-flight execution is lost. This is acceptable for idempotent flows retried by source systems. The audit trail will not contain the incomplete execution.

6. **Performance targets.** 5-step flow: 2–5ms. 10-step flow: 4–10ms. Throughput: ~2,000–5,000 flows/sec per core (conservative).

7. **Metrics.** Lightweight executions are counted separately in the engine metrics. A dashboard widget shows lightweight vs standard throughput.

### BDD Scenarios

```gherkin
Feature: Lightweight Execution Mode

  Scenario: Lightweight workflow executes without per-step DB writes
    Given a workflow with execution_mode "lightweight" and 5 steps
    When the workflow is triggered
    Then no database writes occur during step transitions
    And the workflow completes in under 5ms (excluding step execution time)
    And the full execution trace is written in one batch on completion

  Scenario: Audit trail written on completion
    Given a lightweight workflow that completes successfully
    Then the audit trail contains one workflow completion event
    And one record per step with inputs, outputs, and timing
    And all records share the same transaction timestamp

  Scenario: Reject human task in lightweight mode
    Given a workflow with execution_mode "lightweight" that includes a human task step
    When the workflow is published
    Then validation fails with "human task steps are not allowed in lightweight mode"

  Scenario: Reject SLA timer in lightweight mode
    Given a lightweight workflow with an SLA timer step
    When the workflow is published
    Then validation fails with "SLA timers are not allowed in lightweight mode"

  Scenario: No recovery after crash
    Given a lightweight workflow in progress
    When the Aceryx process crashes
    And restarts
    Then no trace of the in-flight workflow exists
    And no audit record is created for it
    And the source system retries the trigger (e.g. webhook retry)

  Scenario: Error in lightweight mode
    Given a lightweight workflow where step 3 of 5 fails
    Then the workflow stops at step 3
    And the batch audit write includes steps 1, 2, and 3 (with step 3 marked as failed)
    And no partial writes occurred during execution
```

---

## Spec 034 — Opinionated Packs

### Summary

Pre-built solution templates that provide a fast path to value. Each pack is a directory containing workflow YAML, case type schemas, form schemas, plugin requirements, AI component configurations, and sample data. Packs can be deployed via CLI or admin UI.

### Dependencies

- Spec 003 (Case Management) — case types and schemas.
- Spec 009 (Schema-Driven Forms) — form schemas.
- Spec 026 (Plugin Manifest) — plugin requirements.
- Spec 030 (AI Component Registry) — AI component configurations.

### Data Model

```yaml
# Pack structure
/packs/
  loan-origination/
    pack.yaml              # pack metadata
    case-types/
      loan-application.yaml
    workflows/
      origination-flow.yaml
    forms/
      broker-intake.yaml
      underwriter-review.yaml
    ai-components/
      affordability-check.yaml
      fraud-detection.yaml
    sample-data/
      sample-applications.json
    docs/
      README.md
      CUSTOMISATION.md

# pack.yaml
id: loan-origination
name: "Loan Origination Pack"
version: "1.0.0"
description: "End-to-end unsecured business lending workflow"
tier: commercial
category: "Financial Services"
required_plugins:
  - companies-house
  - open-banking
  - experian        # optional, pack works without
required_ai_components:
  - affordability-check
  - fraud-detection
contents:
  case_types: 1
  workflows: 1
  forms: 2
  ai_components: 2
```

### Behaviour

1. **Pack discovery.** Packs are stored in a directory (default `/etc/aceryx/packs/`). The admin UI lists available packs with metadata, required plugins (with availability status), and a "Deploy" button.

2. **Pack deployment.** Deploying a pack creates the case types, imports the workflows, registers the forms, and copies AI component definitions into the tenant's configuration. The deployment is idempotent — re-deploying overwrites with the latest version.

3. **Missing plugin handling.** If a required plugin is not installed, the pack deploys with placeholder steps for the missing connectors. The admin UI shows which plugins need to be added.

4. **Customisation.** Deployed packs are fully editable. The workflow YAML, form schemas, and AI prompts are copied into the tenant's configuration space. Changes do not affect the pack template. The `CUSTOMISATION.md` documents common customisation points.

5. **Sample data.** Packs include sample data that can be loaded to demonstrate the workflow. Sample data creates test cases that exercise the full workflow path.

6. **CLI support.** `aceryx pack list`, `aceryx pack deploy <pack-id>`, `aceryx pack load-samples <pack-id>`.

### BDD Scenarios

```gherkin
Feature: Opinionated Packs

  Scenario: List available packs
    Given 3 packs in the packs directory
    When GET /api/v1/admin/packs is called
    Then 3 packs are returned with metadata
    And each pack shows required plugins and their availability

  Scenario: Deploy a pack
    Given the "loan-origination" pack is available
    And the required plugins "companies-house" and "open-banking" are installed
    When the pack is deployed for a tenant
    Then the "Loan Application" case type is created
    And the "Origination Flow" workflow is imported
    And the intake and review forms are registered
    And the AI components are added to the tenant's registry

  Scenario: Deploy pack with missing plugin
    Given the "loan-origination" pack requires "experian"
    And "experian" is not installed
    When the pack is deployed
    Then the workflow is created with a placeholder step for the Experian check
    And the admin UI shows "Missing plugin: experian — install to enable credit checks"

  Scenario: Idempotent re-deployment
    Given the "loan-origination" pack was deployed previously
    When it is deployed again (newer version)
    Then the case type, workflow, and forms are updated
    And existing cases are unaffected
    And customisations made by the tenant are overwritten (with warning)

  Scenario: Load sample data
    Given the "loan-origination" pack is deployed
    When "aceryx pack load-samples loan-origination" is run
    Then sample loan application cases are created
    And workflows are triggered for each sample case
    And the admin can observe the workflows running end-to-end

  Scenario: Pack customisation
    Given a deployed pack workflow
    When the tenant modifies the workflow (e.g. adds a step)
    Then the modification is saved in the tenant's workspace
    And the original pack template is unchanged
    And future pack updates will warn about overwriting customisations
```

---

# Advanced Compute

---

## Spec 035 — QuantLib WASM Plugin

### Summary

A commercial WASM plugin that embeds QuantLib (the open-source C++ quantitative finance library) compiled to WebAssembly via Emscripten. Exposes a curated facade of 20–30 high-value financial calculation functions as workflow steps.

### Dependencies

- Spec 024 (Plugin Runtime) — WASM execution.
- Spec 026 (Plugin Manifest) — manifest and UI metadata.

### Data Model

```yaml
# manifest.yaml
id: quantlib
name: "QuantLib Financial Calculations"
version: "1.0.0"
type: step
category: "Financial Calculations"
tier: commercial
maturity: core
min_host_version: "1.0.0"

ui:
  icon_svg: |
    <svg viewBox="0 0 24 24">...</svg>
  description: "Industry-standard quantitative finance calculations powered by QuantLib."
  properties:
    - key: function
      label: "Calculation"
      type: select
      options:
        - "bond_price"
        - "swap_npv"
        - "yield_curve"
        - "black_scholes"
        - "var_historical"
        - "var_monte_carlo"
        - "hedge_effectiveness"
        - "fra_rate"
        - "cap_floor_price"
        - "cds_spread"
      required: true
    - key: parameters
      label: "Calculation Parameters"
      type: json
      required: true
      help_text: "JSON object with function-specific parameters. See documentation."

host_functions:
  - host_case_get
  - host_case_set
  - host_log

operational:
  retry_semantics: none
  transaction_guarantee: exactly_once
  idempotent: true
```

### Behaviour

1. **WASM binary.** QuantLib + Boost compiled to WASM via Emscripten. The binary is 15-20MB. It is compiled once at module load time (~50-100ms) and the compiled module is reused.

2. **Facade functions.** The plugin exports a single `Execute` function. The `function` parameter selects the calculation. Each function has a defined parameter schema and output schema. The facade calls the relevant QuantLib objects internally.

3. **Supported functions (v1):**
   - `bond_price` — price a fixed or floating rate bond.
   - `swap_npv` — net present value of an interest rate swap.
   - `yield_curve` — bootstrap a yield curve from market data.
   - `black_scholes` — European option pricing.
   - `var_historical` — Value at Risk from historical returns.
   - `var_monte_carlo` — Value at Risk via Monte Carlo simulation.
   - `hedge_effectiveness` — IFRS 9 hedge effectiveness testing.
   - `fra_rate` — Forward Rate Agreement pricing.
   - `cap_floor_price` — interest rate cap/floor pricing.
   - `cds_spread` — credit default swap spread calculation.

4. **Memory management.** QuantLib C++ objects are allocated and destroyed within a single `Execute` call. No state persists between invocations. The facade handles all QuantLib object lifecycle internally.

5. **Determinism.** Given identical inputs, every function produces identical outputs. No random number generation (Monte Carlo uses a provided seed). This is essential for audit reproducibility.

6. **Audit.** Every calculation is logged with function name, input parameters (as JSON), output, and execution time. The WASM hash is recorded for binary provenance.

### BDD Scenarios

```gherkin
Feature: QuantLib WASM Plugin

  Scenario: Price a fixed-rate bond
    Given the QuantLib plugin is loaded
    And a step configured with function "bond_price"
    And parameters: face_value=1000, coupon_rate=0.05, years_to_maturity=10, yield=0.04
    When the step executes
    Then the result contains a price (approximately 1081.11 for these parameters)
    And execution completes in under 10ms

  Scenario: Bootstrap a yield curve
    Given parameters with market data: deposit rates, swap rates, dates
    When function "yield_curve" is executed
    Then the result contains discount factors and zero rates at each tenor
    And the curve is internally consistent (interpolated rates are smooth)

  Scenario: Monte Carlo VaR with seed
    Given function "var_monte_carlo" with seed=42, portfolio data, and 10000 scenarios
    When executed twice with identical inputs
    Then both executions produce identical VaR results (deterministic)

  Scenario: Hedge effectiveness test
    Given a hedging instrument and hedged item with historical data
    When function "hedge_effectiveness" is executed
    Then the result includes effectiveness ratio, dollar offset, and regression statistics
    And the result indicates whether the hedge qualifies under IFRS 9

  Scenario: Invalid parameters
    Given function "bond_price" with negative face_value
    When the step executes
    Then the result is an error: "invalid parameter: face_value must be positive"

  Scenario: Plugin audit trail
    Given a bond pricing calculation
    When the step completes
    Then the plugin_invocations record includes:
      | Field | Value |
      | plugin_id | quantlib |
      | wasm_hash | SHA-256 of the WASM binary |
      | duration_ms | <10 |
      | status | success |
    And the input_hash allows the calculation to be reproduced

  Scenario: Licence enforcement
    Given the tenant's licence does not include the QuantLib plugin
    When the QuantLib step is added to a workflow
    Then the builder shows a licence warning
    And the workflow cannot be published until the licence is upgraded
```

---

## Spec 036 — Custom Logic Plugin Pattern

### Summary

Defines the pattern for customer-specific WASM plugins that implement proprietary business rules, scoring models, and domain calculations. These plugins receive case data, apply logic, and return decisions — with full audit provenance.

### Dependencies

- Spec 024 (Plugin Runtime) — WASM execution.
- Spec 025 (Plugin SDK) — authoring.
- Spec 026 (Plugin Manifest) — UI configuration.

### Behaviour

1. **Pure computation.** Custom logic plugins use only `host_case_get`, `host_case_set`, and `host_log`. They do not make HTTP calls or access external systems. All data needed for the decision is in the case data.

2. **Decision output.** Every custom logic plugin returns a structured decision:
   ```json
   {
     "decision": "approve|reject|refer|escalate",
     "confidence": 0.85,
     "reasoning": ["Factor 1", "Factor 2"],
     "scores": {"risk": 0.3, "affordability": 0.9},
     "metadata": {}
   }
   ```
   The schema is flexible — `scores` and `metadata` are plugin-specific. The `decision`, `confidence`, and `reasoning` fields are conventional and enable the engine's confidence routing.

3. **Versioned binary provenance.** The audit record for every decision includes the WASM binary hash and manifest version. A regulator can determine exactly which logic version produced a specific decision.

4. **Configuration via properties.** Thresholds and parameters are configured in the manifest's property schema, not hard-coded in the logic. A customer's compliance team can change the credit score threshold from 650 to 700 in the builder without a new WASM build.

5. **Testing harness.** The SDK provides a test runner that loads the WASM plugin, feeds it test case data, and asserts on the decision output. The test runner can be run in CI/CD for regression testing when thresholds or logic change.

6. **Authoring languages.** TinyGo for straightforward rule logic. Rust for compute-intensive scoring models. C/C++ (via Emscripten) for reusing existing customer code.

### BDD Scenarios

```gherkin
Feature: Custom Logic Plugin Pattern

  Scenario: Risk scoring plugin
    Given a custom plugin "lending-risk-score" authored in Rust
    And case data with applicant credit_score=720, loan_amount=30000, employment_years=5
    When the plugin executes
    Then it returns decision="approve", confidence=0.92, scores.risk=0.2

  Scenario: Decision changes with threshold update
    Given the plugin property "min_credit_score" is set to 650
    And an applicant with credit_score=660
    When the plugin executes
    Then the decision is "approve"
    When the property is changed to 700
    And the same case data is re-evaluated
    Then the decision changes to "refer"
    And no WASM rebuild was required

  Scenario: Audit provenance
    Given a decision produced by plugin "lending-risk-score" version "2.1.0"
    When the audit trail is queried
    Then the record includes wasm_hash, manifest version "2.1.0", input data hash, and the full decision output
    And the same WASM binary + same input produces the same decision (deterministic)

  Scenario: Confidence-based routing
    Given the plugin returns confidence=0.5 (below the escalation threshold of 0.7)
    Then the engine creates a human review task
    And the task includes the plugin's reasoning array for reviewer context

  Scenario: Regression testing
    Given a test suite with 50 test cases for the "lending-risk-score" plugin
    When "aceryx plugin test lending-risk-score" is run
    Then all 50 test cases execute against the WASM binary
    And pass/fail results are reported
    And any decision changes from the baseline are flagged

  Scenario: Customer-owned C++ model
    Given a hedge fund's existing C++ risk model compiled to WASM
    And integrated as a custom logic plugin with manifest and property schema
    When case data with trade parameters is provided
    Then the C++ model executes at near-native speed
    And the result is returned as a structured decision
    And execution is sandboxed — the model cannot access other case data or external systems
```

---

## Appendix: Spec Dependency Graph

```
024 Plugin Runtime
 ├── 025 Plugin SDK
 │    └── 026 Plugin Manifest & Registry
 │         ├── 028 HTTP Connector Framework
 │         ├── 029 Trigger Plugin Framework
 │         ├── 030 AI Component Registry
 │         ├── 035 QuantLib WASM Plugin
 │         └── 036 Custom Logic Plugin Pattern
 ├── 027 Core Drivers
 │    └── 029 Trigger Plugin Framework
 ├── 020 AI Assistant
 ├── 021 Code Component
 ├── 022 Channels & Data Ingestion
 │    └── 029 Trigger Plugin Framework
 ├── 023 Document Extraction
 ├── 031 LLM Adapter Framework
 │    ├── 030 AI Component Registry
 │    └── 032 RAG Infrastructure
 ├── 033 Lightweight Execution Mode
 └── 034 Opinionated Packs
```

## Appendix: Implementation Sequence

| Phase | Specs | Rationale |
|---|---|---|
| Phase 2a — Plugin Foundation | 024, 025, 026 | Everything else depends on this |
| Phase 2b — Core Features | 020, 021, 022, 023 | Features needed for lending customer |
| Phase 2c — Integration Layers | 027, 028, 029 | Enable the connector catalogue |
| Phase 2d — AI & Knowledge | 030, 031, 032 | Intelligence features |
| Phase 3 — Advanced | 033, 034, 035, 036 | Performance, packaging, compute |
