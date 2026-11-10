"""Read the settings file a PTY smoke run is mutating without sampling a save.

The PTY suites assert durable settings edits by reading the profile's
settings.json while the binary under test is still running. That reader has no
part in the runtime's save protocol: `saveSettingsPatchReplacing`
(vendor/github.com/MichaelKinsy/PiG/internal/codingagent/settings.go) holds the
profile's directory lock and then rewrites the file with one truncating
`os.WriteFile`, which is how Pi's `fs.writeFileSync` behaves. A reader that
arrives between the truncation and the last chunk therefore sees an empty or
partial file, and parsing that as a failure turns a normal concurrent save into
a spurious test failure -- observed as a `JSONDecodeError` in the model-menu
smoke on the slower macOS runner, while the identical Linux run passed.

Callers here want the file's settled content, not a sample, so wait for the
write to finish instead of racing it. Every read is bounded: content that never
parses is a real failure and is reported as the parse error it is.
"""

import json
import time


def settled_settings(path, timeout=5.0):
    """Return settings.json parsed, waiting out a save that is still being written.

    path is any path-like whose text is JSON. timeout bounds the wait in seconds;
    a file that stays unparseable for that long raises the JSONDecodeError
    unchanged, because that is corruption the suite must not paper over. The
    normal case, a settled file, parses on the first read and neither sleeps nor
    waits. A caller that polls this from an output-draining loop (the PTY helpers
    re-evaluate their predicate on every packet) blocks only until the in-flight
    save completes.
    """
    deadline = time.monotonic() + timeout
    while True:
        try:
            return json.loads(path.read_text())
        except json.JSONDecodeError:
            if time.monotonic() >= deadline:
                raise
            time.sleep(0.005)
