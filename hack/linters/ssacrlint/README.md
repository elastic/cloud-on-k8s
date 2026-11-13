# ssacrlint

A [go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis) linter. It flags direct calls to `client.Writer.Update()` and implicit-ownership `client.Writer.Patch()` calls on ECK CRs. These calls can cause Server-Side Apply (SSA) field-ownership conflicts.

## Background

ECK manages Kubernetes resources with [Server-Side Apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/). SSA records which manager owns which field. A full-object `Update`, or a full-object merge `Patch`, claims ownership of every field in the object it sends.

Another manager can own some of those fields. Helm, a user's `kubectl apply`, and other controllers are examples. The write then takes that ownership away, or it causes a conflict.

Some controllers change only a few fields of an object. For those controllers, the safe pattern is a scoped patch that touches only those fields. This linter finds the unscoped writes at compile time, before the regression reaches a cluster.

## What is flagged

The linter reports a call when the receiver implements `client.Writer` and:

- The method is `Update`: `c.Update(ctx, obj)`, where `obj` is an ECK CR or can be one.
- The method is `Patch` **and** the patch type claims fields implicitly:
  - `client.MergeFrom`, `client.MergeFromWithOptions`, `client.StrategicMergeFrom`
  - `client.RawPatch(types.MergePatchType, …)`, `client.RawPatch(types.StrategicMergePatchType, …)`
  - Any patch type the analyzer cannot resolve statically (conservative)

Explicit-field patch types are **not** flagged: `client.RawPatch(types.JSONPatchType, …)`, `client.RawPatch(types.ApplyPatchType, …)`, `client.RawPatch(types.ApplyCBORPatchType, …)`, and `client.Apply`. These only touch the fields they name and cannot claim unintended ownership.

The linter does not flag sub-resource writes such as `c.Status().Update(...)` and `c.Status().Patch(...)`. `SubResourceWriter` has different method signatures, so it does not implement `client.Writer`. The exclusion follows from the type, not from a check on the method name.

The linter does not flag calls on plain Kubernetes types such as `corev1.Secret` and `appsv1.StatefulSet`.

### Type resolution

At a call site, the static type is often the `client.Object` interface and not the concrete type. The linter therefore uses SSA (Static Single Assignment) form to trace the value back to its origin:

- `*ssa.MakeInterface` shows the concrete type that goes into the interface. This covers ordinary assignments, explicit conversions such as `client.Object(es)`, and parenthesised forms.
- `*ssa.Phi` nodes join values from conditional branches. If every incoming edge is an ECK CR, the linter flags the call as a CR write. If at least one edge is a CR and another is not, the linter emits the mixed-branch diagnostic. If no edge is a confirmed CR, the linter reports the call as unresolved.
- Some values cannot be traced inside the package. Function parameters typed as `client.Object`, results from other functions, and cross-function flows are examples. The linter reports these as unresolved.

The linter does not unwind cross-function flows. An ECK CR can pass through a `client.Object` parameter into a helper that calls `Update`. The unresolved diagnostic then appears at the helper, not at the caller.

## Diagnostics

The linter emits one of three messages. For `Patch` calls the suffix names the safe alternatives; for `Update` it does not (a full-object replace has no targeted equivalent).

The concrete type resolves to an ECK CR:

```
client.Writer.Update() on an ECK CR may cause SSA field-ownership conflicts. Add //nolint:ssacrlint to mark this as safe and dismiss this report.
client.Writer.Patch() on an ECK CR may cause SSA field-ownership conflicts. Use a scoped JSON Patch or client.Apply, or add //nolint:ssacrlint if this full-object write is intentional.
```

The argument is interface-typed and is an ECK CR on at least one conditional branch:

```
client.Writer.Update() has an interface-typed argument that is an ECK CR on at least one branch. This call may cause SSA field-ownership conflicts depending on which branch runs. Add //nolint:ssacrlint to mark this as safe and dismiss this report.
client.Writer.Patch() has an interface-typed argument that is an ECK CR on at least one branch. This call may cause SSA field-ownership conflicts depending on which branch runs. Use a scoped JSON Patch or client.Apply, or add //nolint:ssacrlint if this full-object write is intentional.
```

The argument is interface-typed and the concrete type is unknown:

```
client.Writer.Update() has an interface-typed argument. The analyzer cannot resolve its concrete type. If the value is an ECK CR, this could cause SSA field-ownership conflicts. Add //nolint:ssacrlint to mark this as safe and dismiss this report.
client.Writer.Patch() has an interface-typed argument. The analyzer cannot resolve its concrete type. If the value is an ECK CR, this could cause SSA field-ownership conflicts. Use a scoped JSON Patch or client.Apply, or add //nolint:ssacrlint if this full-object write is intentional.
```

The second and third messages are deliberately conservative. A mixed or unresolved argument can be an ECK CR, so the linter asks for a confirmation instead of staying silent.

## Suppression

Some full-object writes are intentional. One example is a controller that owns the whole spec. Mark those writes with a `//nolint:ssacrlint` directive and a short reason.

Suppression is handled by golangci-lint's own nolint processing. These forms work:

```go
// Same line as the call
c.Update(ctx, es) //nolint:ssacrlint // legitimate full-object write: this controller owns the whole spec

// Standalone comment on the line immediately before the call
//nolint:ssacrlint
c.Update(ctx, es)

// Combined with other linters
c.Update(ctx, es) //nolint:govet,ssacrlint

// Line that opens a multi-line call
c.Update( //nolint:ssacrlint
    ctx,
    es,
)
```

One form does not work:

- A directive on an argument line of a multi-line call. The linter reports the diagnostic at the line that opens the call, so golangci-lint does not match a directive below that line.

When running the standalone binary (`go vet -vettool=...`), there is no suppression mechanism. Every flagged call produces a diagnostic regardless of any `//nolint` comment.

### Repository-level exclusions

`.golangci.yml` disables the linter for `*_test.go` files and for `test/e2e/`. Full-object writes are expected in both.

## Running the linter

### golangci-lint plugin (primary mode)

The linter runs as a [module plugin](https://golangci-lint.run/docs/plugins/module-plugins/). The build produces a custom golangci-lint binary that embeds the analyzer:

```bash
make lint
```

The `lint` target does two things. It builds `./bin/golangci-lint-custom` from `.custom-gcl.yml`. It then runs the full lint suite.

The target rebuilds the binary when the sources, `go.mod`, or `go.sum` of the linter change.

To build the custom binary alone:

```bash
golangci-lint custom
```

### Standalone binary

`cmd/main.go` wraps the analyzer in `singlechecker` and produces a `go vet` compatible binary. It runs the analyzer alone, without the issue processors, path filters, and `.golangci.yml` exclusions of golangci-lint.

Use it to answer one question: does the analyzer report this, or does golangci-lint filter it? That question matters when a suppression does not work as expected. It also matters when you check a new detection case against real ECK packages.

```bash
# Build the binary (from this directory)
go build -o /tmp/ssacrlint ./cmd/main.go

# Run it against ECK packages (from the repository root)
go vet -vettool=/tmp/ssacrlint -tags release ./pkg/... ./cmd/...
```

Always pass `-tags release`. ECK puts some code behind that build tag. Without the tag, the analyzer reads a different set of files than CI does.

No Makefile target and no CI step use this binary, and that is deliberate. It is a debugging aid for people who work on the analyzer. For everyone else, `make lint` is the supported entry point.

## Configuration

The analyzer accepts one flag:

| Flag | Default | Purpose |
|------|---------|---------|
| `-cr-path-pattern` | `` `^github\.com/elastic/cloud-on-k8s/(v\d+/)?pkg/apis/` `` | Regexp matched against package paths to identify ECK CRD types. |

The default matches `pkg/apis/` packages under the ECK module root, including the major-version layout (`v3/pkg/apis/…`) used when the module is bumped to v3 or later. The flag accepts any valid Go regexp, so the pattern can be overridden directly without any string-assembly logic in the analyzer.

The flag is useful in two situations: in the test suite, where it points the analyzer at an in-module fake API package under `testcases/fakeapi/pkg/apis/` so that tests do not depend on the real ECK module; and when running the linter on a fork or a codebase where ECK CRs live under a different module path.

The flag can be set through `.golangci.yml` via the `settings:` sub-key of the custom linter block:

```yaml
linters:
  settings:
    custom:
      ssacrlint:
        type: module
        description: "..."
        settings:
          cr-path-pattern: '^github\.com/your-fork/cloud-on-k8s/(v\d+/)?pkg/apis/'
```

golangci-lint passes that block to `New(settings any)` in `plugin/plugin.go`, which reads the `cr-path-pattern` key and forwards the value to the analyzer flag. The standalone binary accepts the flag directly:

```bash
go vet -vettool=/tmp/ssacrlint -ssacrlint.cr-path-pattern='^github\.com/...' ./...
```

## Development

Run the unit tests of the linter from the repository root:

```bash
make ssacrlint-unit-tests
```

The target also runs `go mod tidy`, because this linter is a separate Go module and the top-level `tidy` target does not reach it. In CI the step pairs the target with `check-local-changes`, which fails the build if `go mod tidy` changed `go.mod` or `go.sum`.

Pass extra `go test` flags through `TEST_OPTS`:

```bash
make ssacrlint-unit-tests TEST_OPTS="-run TestAnalyzer -v"
```

### Adding a test case

Test fixtures are in `testcases/testcases.go`. To add one:

1. Add an exported function. Name it after the behaviour it exercises and prefix the name with `Flagged` or `NotFlagged`.
2. Write a doc comment. State what the analyzer must do, and why.
3. For a case that must produce a diagnostic, add a `// want "..."` annotation on the same line as the `Update` or `Patch` call:
   - `// want "on an ECK CR"` — concrete ECK CR type resolved.
   - `// want "on at least one branch"` — interface-typed argument that is an ECK CR on at least one conditional branch.
   - `// want "cannot resolve its concrete type"` — interface-typed argument whose concrete type is unresolvable.

A case that must not be flagged needs no annotation. The test driver (`analysistest`) fails on any diagnostic that has no matching annotation, and on any annotation that has no matching diagnostic.

New fixtures can be added anywhere in the file — annotations travel with the code and no other file needs updating.
