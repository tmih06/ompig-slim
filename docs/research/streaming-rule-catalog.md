# Streaming-rule catalog research

## Goal and method

The goal is to catch recognizable correctness, security, maintenance, and
performance hazards while the agent writes code. This report records the
primary-source evidence and the limits of the selected regex checks.

The existing engine checks added source snapshots, not a parsed or type-checked
program. Its contract is documented in [Streaming rules](../streaming-rules.md).
The design decision is to keep every bundled rule advisory. A match asks for
inspection, not an automatic rewrite or a claim that the program is defective.

Research ran locally because this session exposes no background-agent tool.
Sources below are language and library documentation, official lint documentation
or implementation, and Mozilla's API reference. No secondary blog post is used
as the basis for a check. The corrective text is written for this project rather
than copied from the source.

## Selected additions

### Go

| Rule | Finding and primary evidence | Detection boundary and exception |
|---|---|---|
| `go-regexp-loop` | Package-level regex matching compiles on every call. Repeating a constant pattern in a loop repeats compilation. [Staticcheck SA6000 source](https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa6000/sa6000.go) documents the cost. | Looks for a nearby regex compilation or package-level match inside a loop. It cannot prove loop nesting or whether a pattern changes. Cache only stable patterns. |
| `go-defer-loop` | Deferred cleanup runs when the function returns, not after each iteration. [Staticcheck SA9001 source](https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa9001/sa9001.go) explains accumulation in channel range loops. | Looks for nearby `defer` inside a loop. Bounded loops and cleanup intentionally delayed to function return can be correct. Nested function boundaries need inspection. |
| `go-waitgroup-add` | Calling `WaitGroup.Add` inside the new goroutine can race with `Wait`. [Staticcheck SA2000 source](https://raw.githubusercontent.com/dominikh/go-tools/master/staticcheck/sa2000/sa2000.go) requires registration before starting that goroutine. | Matches an immediately launched closure whose first statement is a receiver's `Add`. The receiver's type is unknown. `Add` on a non-WaitGroup is not necessarily wrong. |
| `go-http-client-timeout` | A zero `http.Client.Timeout` means no timeout. [Go HTTP client source](https://raw.githubusercontent.com/golang/go/master/src/net/http/client.go) defines that contract. | Checks an empty `http.Client{}` literal. A request context, later configuration, or intentionally long-lived stream can already supply the correct lifetime. |
| `go-tls-skip-verify` | `InsecureSkipVerify` disables certificate-chain and hostname verification unless custom verification replaces it. [Go TLS Config source](https://raw.githubusercontent.com/golang/go/master/src/crypto/tls/common.go) explains the interception risk and allowed verification callbacks. | Checks an explicit true field or assignment. Tests and sound `VerifyConnection` or `VerifyPeerCertificate` implementations can be legitimate. |
| `go-ignored-json-error` | `json.Unmarshal` returns syntax and type errors and can leave partially decoded data. [Go JSON decoder source](https://raw.githubusercontent.com/golang/go/master/src/encoding/json/decode.go) defines those outcomes. | Checks explicit `_ = json.Unmarshal(...)`. Trusted constants and deliberate best-effort parsing require justification, not fabricated fallback data. |
| `go-sql-format` | Formatting input into SQL risks injection. Pass data separately from the SQL statement. [Go database guide](https://go.dev/doc/database/sql-injection) provides the unsafe and parameterized forms. | Checks direct `fmt.Sprintf` arguments to Query or Exec methods. There is no taint analysis. Validated identifiers and non-database methods can be exceptions. |

### JavaScript and TypeScript

The `js-` checks also apply to TypeScript source. They recognize syntax shared
by JavaScript and TypeScript rather than enforcing a particular framework.

| Rule | Finding and primary evidence | Detection boundary and exception |
|---|---|---|
| `js-async-promise-executor` | An async Promise executor's thrown error does not reject the outer Promise. [ESLint's official rule](https://eslint.org/docs/latest/rules/no-async-promise-executor) documents the lost-error behavior. | Checks `new Promise(async ...)`, including simple generic annotations. Direct async functions are not flagged. |
| `js-async-foreach` | `Array.forEach` does not await returned Promises. [Mozilla's forEach reference](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/forEach) states that callbacks are synchronous. | Checks an async callback passed directly to `.forEach`. Deliberate detached work still needs an explicit error and lifetime policy. Other APIs named forEach may differ. |
| `js-empty-catch` | Empty blocks can be incomplete refactors or hidden failures. [ESLint no-empty](https://raw.githubusercontent.com/eslint/eslint/main/docs/src/rules/no-empty.md) permits an explanatory comment in an intentional empty catch. | Checks whitespace-only catch bodies. A comment explaining intentional suppression is not matched. |
| `js-sync-io` | Synchronous filesystem and process APIs block Node's event loop and are not intended for server request paths. [Node's official guide](https://nodejs.org/en/learn/asynchronous-work/dont-block-the-event-loop) gives the API families and context. | Checks selected synchronous API call names. Startup, build tools, short CLI scripts, and worker threads can use them intentionally. It does not claim every sync call is a performance bug. |
| `js-reduce-spread` | Repeatedly copying a growing reduce accumulator can introduce quadratic work. [Mozilla's reduce guidance](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/reduce#when_to_not_use_reduce) gives the mechanism and alternatives. | Checks a direct arrow reducer that spreads its own accumulator into a new object or array. Small bounded collections can be fine. Mutate only an accumulator owned by this operation, never shared input. |
| `js-json-clone` | JSON serialization omits unsupported values, changes Date values, and rejects cycles. [JSON.stringify reference](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/JSON/stringify) defines those semantics. [structuredClone](https://developer.mozilla.org/en-US/docs/Web/API/Window/structuredClone) supports deep copies and cycles. | Checks adjacent `JSON.parse(JSON.stringify(...))`. Intentional JSON normalization is valid. structuredClone is not a universal replacement for functions, class instances, or every runtime. |
| `js-map-parseint` | `map(parseInt)` passes the index as the radix and can produce NaN. [Mozilla's map reference](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Array/map#using_parseint_with_map) demonstrates the argument mismatch. | Checks a bare parseInt callback, including `Number.parseInt`. An explicit callback with a radix is not flagged. |
| `ts-check-suppression` | `@ts-ignore` and `@ts-nocheck` suppress compiler diagnostics. [typescript-eslint's official rule](https://typescript-eslint.io/rules/ban-ts-comment/) documents those directives and descriptive expect-error usage. [TypeScript 3.9](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-3-9.html) explains that expect-error reports stale suppressions. | Checks those directives only in comments. Generated files, compiler-version compatibility, and proven library typing errors can justify narrow suppressions. An expect-error directive is not itself a fix for unsafe code. |

### Python

| Rule | Finding and primary evidence | Detection boundary and exception |
|---|---|---|
| `py-mutable-default` | Mutable argument defaults survive between calls. [Ruff B006](https://docs.astral.sh/ruff/rules/mutable-argument-default/) explains shared state and intentional-cache exceptions. | Checks simple list, dict, or set defaults in a single-line signature. Does not resolve aliases or all multiline signatures. Use None only when it preserves the API's semantics. |
| `py-bare-except` | Bare except catches BaseException, including interruption and exit signals. [Ruff E722](https://docs.astral.sh/ruff/rules/bare-except/) explains the consequences. | Checks an unqualified except header. Cleanup that immediately re-raises can be deliberate. Catch the expected exception rather than blindly replacing it with a broad Exception handler. |
| `py-swallowed-exception` | Broad except-pass can hide failures. [Ruff S110](https://docs.astral.sh/ruff/rules/try-except-pass/) recommends preserving useful failure evidence. | Checks an Exception or BaseException handler followed directly by pass. Explicit suppression of a specific expected exception is not flagged. |
| `py-subprocess-shell` | shell=True requires the caller to handle shell metacharacters safely. [Python subprocess security considerations](https://docs.python.org/3/library/subprocess.html#security-considerations) describes injection risk and the Windows batch-file exception. | Checks a direct subprocess call with explicit shell=True. Controlled shell syntax can be necessary. Do not change Windows batch execution without checking its quoting behavior. |
| `py-pickle-load` | Unpickling attacker-controlled data can execute arbitrary code. [Python pickle documentation](https://docs.python.org/3/library/pickle.html) warns to unpickle only trusted data. | Checks pickle.load and loads calls. This is a trust-boundary review prompt, not proof that the argument is untrusted. Trusted local persistence can remain. |
| `py-yaml-load` | The YAML loader can construct Python objects. [PyYAML's documentation](https://pyyaml.org/wiki/PyYAMLDocumentation) warns against loading untrusted data and points to safe_load. | Checks yaml.load calls without an explicit SafeLoader or CSafeLoader in the simple argument list. Safe custom loaders need inspection. |
| `py-requests-timeout` | Requests does not time out unless a timeout is supplied. [Requests quickstart](https://requests.readthedocs.io/en/latest/user/quickstart/#timeouts) distinguishes socket inactivity from a total download deadline. | Checks simple direct requests calls without an explicit timeout. Nested expressions are deliberately skipped rather than guessing where the call ends. A configured wrapper or deliberate stream can be valid. |
| `py-dynamic-exec` | eval and exec execute Python code. [Python built-in function documentation](https://docs.python.org/3/library/functions.html#exec) warns about untrusted input. [Ruff S307](https://docs.astral.sh/ruff/rules/suspicious-eval-usage/) also documents eval. | Checks unqualified eval or exec calls at a statement boundary. Trusted code-generation tools can be valid. Prefer a bounded parser for data. [CPython's literal_eval implementation](https://raw.githubusercontent.com/python/cpython/main/Lib/ast.py) warns that complex expressions can overflow the C stack, so it is not a resource sandbox. |

### Rust

| Rule | Finding and primary evidence | Detection boundary and exception |
|---|---|---|
| `rs-mem-forget` | mem::forget skips destruction and may leak resources. [Rust's official documentation](https://doc.rust-lang.org/std/mem/fn.forget.html) also documents legitimate FFI use and ManuallyDrop. | Checks qualified mem::forget. It is safe Rust and is not automatically undefined behavior. Ownership transfer must preserve the external owner's contract. |
| `rs-unwrap-unchecked` | Option::unwrap_unchecked on None is undefined behavior. [Rust Option safety contract](https://doc.rust-lang.org/std/option/enum.Option.html#method.unwrap_unchecked) requires a valid variant. | Checks the method call, including similarly named methods on Result. Prefer checked control flow unless an independently proven invariant justifies unsafe code. |
| `rs-uninitialized` | mem::uninitialized is deprecated and causes immediate undefined behavior for nearly all types. [Rust's official documentation](https://doc.rust-lang.org/std/mem/fn.uninitialized.html) recommends MaybeUninit. | Checks qualified calls and simple generic forms. MaybeUninit still requires correct initialization before assume_init. |
| `rs-collect-count` | Collecting into Vec just to get its length can be replaced with count. [Clippy's implementation](https://raw.githubusercontent.com/rust-lang/rust-clippy/master/clippy_lints/src/methods/needless_collect.rs) makes that recommendation and explicitly checks side effects and type constraints. | Checks explicit collect::<Vec<_>>().len()-style chains. Regex cannot reproduce Clippy's type and ownership checks. Preserve destructor timing, side effects, and type inference. |
| `rs-unchecked-utf8` | from_utf8_unchecked requires valid UTF-8. [Rust's official safety contract](https://doc.rust-lang.org/std/str/fn.from_utf8_unchecked.html) states the precondition. | Checks str and String conversion forms. Independently validated internal bytes can justify unsafe conversion. External data should use checked conversion and handle errors. |

## Retained checks

The six existing checks remain. Their implementation and examples are in
`internal/streamingrules/builtin` and
`internal/streamingrules/testdata/builtin-cases.json`.

| Rule | Primary source |
|---|---|
| `go-ioutil`, `go-rand-v2`, `go-join-hostport` | The original oh-my-pi rule sources at commit `1bdbf6d242d99ca62bdf2feeff4519ff992f647d`, under `packages/coding-agent/src/discovery/builtin-rules`, inspected during the initial port. Their attribution is in `NOTICE`. |
| `ts-no-any`, `ts-bare-catch`, `rs-box-leak` | The same pinned oh-my-pi rule sources. Their existing caveats and advisory delivery remain unchanged. |

## Deliberate exclusions

- No universal ban on time.Tick. [Current Go source](https://raw.githubusercontent.com/golang/go/master/src/time/tick.go) states that Go 1.23 and later collect unreferenced tickers and removes the old efficiency preference for NewTicker.
- No blanket ban on unwrap, clone, unsafe, dynamic import, tiny functions, nested loops, or Promise.all. These tokens alone do not establish defective code. The selected Rust rules target explicit unsafe preconditions or recognizable wasted collection instead.
- No generic SQL-injection, authentication, race, N+1 query, missing-await, or ownership verdict. The engine lacks types, call graphs, runtime input bounds, and taint analysis. The SQL and WaitGroup checks only recognize specific syntax worth reviewing.
- No measured speedup claim. The performance rules identify mechanisms described by their owning sources. Their impact depends on input size, execution frequency, and the application. Benchmark the changed application before claiming an improvement.
- No automatic edits, blocking built-ins, model judge, AST dependency, or new matcher options. The selected files use the existing Markdown contract and once-per-run reminders.

## Verification contract

`internal/streamingrules/testdata/builtin-cases.json` is the explicit coverage matrix. Every bundled
rule must have matching examples, non-matching alternatives, valid file scopes,
and a summary. The tests reject untested rule names and fixtures without a
corresponding bundled rule. They exercise decoded write and edit snapshots,
ignore deleted code and documentation files, and keep all built-ins advisory.

The real-binary smoke fixture exercises new rule reminders, saved poke data,
actual writes, once-per-run delivery, and disabled rules across the four language
families. TUI runs verify summaries in painted frames and after session resume.
These checks prove detection and delivery. They do not prove that a live model
follows the correction or that the resulting program is secure or faster.
