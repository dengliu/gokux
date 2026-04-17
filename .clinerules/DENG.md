## Code update rules
- Activate virtual env via `source .venv/bin/activate`
- After updating the code, alwasy Run `ruff format --check sesame` and `ruff check sesame` and fix the error
- Follow this rule to document common library: https://realpython.com/documenting-python-code/

## Tool Use Rule: Automatic Activation

### Tool Use Safety
- **Explicit Paths:** When using `read_file` or `edit_file`, you MUST explicitly provide the `path` parameter.
- **No Empty Args:** Never call a tool with empty arguments. If you are unsure of the path, use `list_files` first to confirm it.
This rule defines when to automatically activate MCP server tools. Adhere to these guidelines to select the most relevant tools for the current task.

### Trigger conditions

- **Use Context7 MCP:** If the user's request involves a specific technology, library, or framework that is not part of the standard LLM knowledge base or requires up-to-date documentation, you MUST use the Context7 MCP.

- **Use GitHub MCP:** If the user's request involves any of the following, you MUST use the GitHub MCP:
    - Interacting with GitHub issues or pull requests.
    - A url of github action run. e.g `https://github.com/SesameAILabs/sesame/actions/runs/19492095259/job/55786133472?pr=10503`
    - Checking the status of a specific pull request or issue.
    - Creating, updating, or closing issues.
    - Listing a repository's branches, pull requests, or issues.
    - Do not use emoji when updating or commennting PRs.
    - When reviewing code and adding comments to a GitHub PR, do not use Unicode characters or emoji. Add review comments inline in PENDING state.
    - **Addressing PR review comments:** When replying to PR review comments, you MUST reply directly within each existing review thread — never post a single aggregated issue comment or standalone comment. Use the GitHub API's reply-to-comment endpoint (`POST /repos/{owner}/{repo}/pulls/{pull_number}/comments/{comment_id}/replies`) via `gh api` to post each reply as a threaded response under the original review comment. This ensures reviewers see responses inline next to their feedback.

- **Use Kubernetes MCP:** If the user's request explicitly mentions or implies interaction with a Kubernetes cluster, you MUST use the Kubernetes MCP. Triggers include:
    - Managing pods, deployments, or services.
    - Checking the status of Kubernetes resources.
    - Deploying, updating, or deleting manifests.
    - Commands like `kubectl get`, `kubectl apply`, `kubectl explain` or `helm install`.
    - Use `kubectl explain` to get the full schema details for tasks that are related to CRD
    - use the current kubernetes context if not expecifitly specified in the prompt

- **Use Git Tools MCP:** If the user's request involves more advanced Git operations beyond the core Git MCP, you MUST use the Git Tools MCP. This includes:
    - Checking the current branch.
    - Staging, committing, or reverting changes.
    - Pulling or pushing to a remote repository.
    - Reviewing the commit history.
    - Merging or rebasing branches.
    - Resolving merge conflicts.
    - Interacting with submodules.
    - Performing complex repository-level actions.
    - Before push commits to remote, alwasy Run `ruff format --check sesame` and `ruff check sesame`, fix the error


- **Use Linear MCP:** If the user's request involves managing issues, tasks, or projects within the Linear app, you MUST use the Linear MCP. This is triggered by references to:
    - Linear issues, projects, or teams.
    - Creating, updating, or querying Linear tickets.
    - Keywords like "linear issue" or "Linear project."


- **Use Time MCP:** If the user's request involves date, time, or scheduling, you MUST use the Time MCP. This includes:
    - Getting the current time.
    - Calculating time differences.
    - Formatting dates or timestamps.
    - Scheduling events or reminders.

- **JetBrains MCP:** When you have access to the JetBrains MCP proxy. Please adhere to the following workflow rules to utilize the IDE's capabilities effectively:

    - 1. Code Navigation & Discovery
      **Prefer IDE Search:** When looking for classes, methods, or interfaces, prioritize using JetBrains' intelligent code search and indexing tools over standard file reads or basic grep searches.
      **Context Gathering:** If a user asks a broad question about how a feature works, use the IDE tools to map out the relevant file structure and class relationships before writing code.

    - 2. Refactoring & Modifications
      **Find Usages First:** Before changing a function signature, renaming a variable, or deleting a method, you MUST use the JetBrains tools to find all usages across the project to prevent breaking changes.
      **Symbol Resolution:** If you encounter unknown symbols or variables, use the IDE's resolution capabilities to figure out where they are imported from rather than guessing.

    - 3. Code Quality & Formatting
      **Run Inspections:** After implementing a complex feature or refactoring code, utilize IntelliJ's code inspection tools to check for warnings, errors, or optimization suggestions.
      **Format Before Completion:** Before finalizing a task, trigger the IDE's formatting tools on the modified files to ensure the code matches the project's established style guidelines.

- **Use gcloud CLI tool:** If the user's request involves interacting with, provisioning, or querying Google Cloud Platform (GCP) resources, you MUST use the gcloud command line. This includes:
  - Setting or verifying the active GCP project and configuration.
  - Deploying applications or services (e.g., to Cloud Run, App Engine, or Cloud Functions).
  - Managing infrastructure like Compute Engine VMs, Kubernetes Engine (GKE) clusters, or Cloud Storage buckets.
  - Configuring Identity and Access Management (IAM) roles, permissions, and service accounts.
  - Fetching logs, metrics, or status information for running cloud services. 

### Tool usage guidelines

- **Do not specify tools unless necessary:** Do not write `@use-tool` commands directly in your responses. The instructions above are sufficient for you to decide which tools to use.
- **Explain tool usage:** When using an MCP, briefly mention which tools are being used and why before providing the final answer. This helps the user understand your process.

### Example scenarios
- **User request:** “Delete this temporary log file in `/tmp/test.log`.”  
  → Use **File System MCP** (outside IDE project scope).

- **User request:** “Update the Kubernetes deployment in the Kubernetes cluster for my app.”  
  → Use **Kubernetes MCP** (cluster-related, not local code).

- **User request:** “What’s today’s date?”  
  → Use **Time MCP**.  

## Go Lint Rules
- Before every commit, you MUST run the following lint command and fix all errors:
```bash
docker run --rm \
    -v $(PWD):/app \
    -v $(HOME)/go/pkg/mod:/root/go/pkg/mod:ro \
    -w /app \
    golangci/golangci-lint:latest \
    golangci-lint run ./...
```
- All lint errors MUST be resolved before committing. Do not commit code with lint failures.
- This applies to all Go code changes in the repository.

## Golang code rule
- Use go1.26 for coding and code review
- Never use deprecated Go functions. If a function has a `// Deprecated:` comment, always use the recommended replacement.
- "Specifically enforce these replacements in Go code:"
  - "`grpc.Dial` → `grpc.NewClient`"
  - "`ioutil.ReadAll` → `io.ReadAll`"
  - "`ioutil.ReadFile` → `os.ReadFile`"
  - "`ioutil.WriteFile` → `os.WriteFile`"
  - "`ioutil.ReadDir` → `os.ReadDir`"
  - "`ioutil.NopCloser` → `io.NopCloser`"
  - "`ioutil.TempFile` → `os.CreateTemp`"
  - "`ioutil.TempDir` → `os.MkdirTemp`"

## Rule to Enforce OpenTelemetry Semantic Conventions

### 1. General Attribute Naming Conventions

All generated telemetry or Datadog instrumentation **must strictly adhere** to the official [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/) and [Semantic Convention Specifications](https://opentelemetry.io/docs/specs/semconv/):

* **Case:** All attribute names **must** be in `snake_case`.
* **Namespace:** Attributes **must** be namespaced using dots (`.`) to group related attributes. For example, `http.request.method` is preferred over `http_request_method`.
* **No Redundancy:** Avoid redundant prefixes. For example, use `http.request.method` instead of `http.request.request_method`.
* **Clarity and Conciseness:** Attribute names should be clear, concise, and easy to understand.

This applies to:
- Traces (spans, events, links)
- Metrics (names, units, attributes)
- Logs (bodies, attributes, severity)
- Profiles
- Resource attributes
- Any custom telemetry attributes

### 2. Resource Semantic Conventions

Resource attributes describe the entity producing the telemetry.

* **`service.name`:** This is a **required** attribute that logically identifies the service.
* **`service.namespace`:** Used to group services together, for example, `production` or `staging`.
* **`service.instance.id`:** A unique identifier for the instance of the service.
* **`service.version`:** The version of the service.
* **Cloud Provider Attributes:** When running on a cloud platform, resource attributes **should** include information about the provider, region, and instance, for example `cloud.provider`, `cloud.region`, and `cloud.instance.id`.
* **Technology Attributes:** Include attributes that identify the technology stack, such as `process.runtime.name` and `process.runtime.version`.

### 3. Trace Semantic Conventions
Traces and spans provide insights into the lifecycle of a request.

* **Span Names:** Span names **should** be low-cardinality, meaning they should not contain unique identifiers. A good span name describes the operation being performed, for example, `HTTP GET` or `database query`.
* **Span Kind:** The `span.kind` attribute is **required** and indicates the type of operation. It **must** be one of `SERVER`, `CLIENT`, `PRODUCER`, `CONSUMER`, or `INTERNAL`.
* **HTTP Spans:** For HTTP client and server spans, the following attributes are **required**:
    * `http.request.method`
    * `url.full`
    * `http.response.status_code`
* **Database Spans:** For database client spans, the following attributes are **required**:
    * `db.system`
    * `db.statement` (for SQL databases)
* **Error Reporting:** When an error occurs, the `error` attribute **must** be set to `true`, and the `error.description` attribute **should** contain a description of the error.

### 4. Metric Semantic Conventions

Metrics provide quantitative measurements of your application's performance.

* **Metric Names:** Metric names **must** be in `snake_case` and follow a `namespace.instrument` structure. For example, `http.server.request.duration`.
* **Units:** The unit of measurement **must** be included in the metric name where applicable, following the Unified Code for Units of Measure (UCUM). For example, `ms` for milliseconds, `by` for bytes.
* **Counter Naming:** Counters that are monotonically increasing **should** have a `.total` suffix.
* **HTTP Metrics:** Standard HTTP metrics **should** be used, such as `http.server.active_requests` and `http.client.request.duration`.
* **System Metrics:** System-level metrics **should** be prefixed with `system.`, for example, `system.cpu.utilization`.


### 5. Log Semantic Conventions

Logs capture discrete events from your application.

* **Log Body:** The primary content of the log record **must** be in the `body` field.
* **Severity:** Log severity **must** be recorded using `severity.text` (e.g., `INFO`, `ERROR`) and `severity.number`.
* **Trace and Span Context:** Logs **should** be associated with a trace and span by including `trace_id` and `span_id` when available.
* **Attributes:** Use attributes to provide structured context to your logs. For example, instead of logging `"User 123 logged in"`, use a log with the body `"User logged in"` and an attribute `user.id: "123"`.

### 6. Profiling Semantic Conventions

Profiles provide insights into the resource consumption of your code.

* **Profile Naming:** Profiles should be named to reflect the type of data they contain, for example, `cpu_profile` or `memory_profile`.
* **Sample Types:** The `sample.type` attribute **must** indicate the type of data being sampled (e.g., `cpu`, `memory`).
* **Units:** The `sample.unit` attribute **must** specify the unit of measurement for the sampled data.

