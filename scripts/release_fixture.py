"""Release fixtures shared by the install and self-update suites.

Both suites need the same two things: an archive in exactly the layout
scripts/build-binaries.sh publishes, and a loopback server exposing GitHub's
release URL shape. Keeping the layout in one place means a change to the
release contract is proven by both suites at once, instead of each drifting
until one of them stops resembling what the build actually ships.
"""

import hashlib
import http.server
import io
import json
import pathlib
import tarfile
import threading

# ARCHIVE_MEMBER is the executable's path inside a release archive.
# Keep it in step with scripts/build-binaries.sh.
ARCHIVE_MEMBER = "ompig-slim/ompig-slim"


def sha256_bytes(data):
    """Return the lowercase hex SHA-256 of data."""
    return hashlib.sha256(data).hexdigest()


def archive_bytes(payload, compresslevel=9):
    """Wrap one executable payload in the release archive layout.

    The archive also carries a README, as a real release does, so a test can
    tell "extract only the executable" apart from "extract everything".
    """
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz", compresslevel=compresslevel) as package:
        member = tarfile.TarInfo(ARCHIVE_MEMBER)
        member.size = len(payload)
        member.mode = 0o755
        package.addfile(member, io.BytesIO(payload))
        notice = tarfile.TarInfo("ompig-slim/README.md")
        notice.size = 3
        notice.mode = 0o644
        package.addfile(notice, io.BytesIO(b"doc"))
    return buffer.getvalue()


def archive_of(members):
    """Build an archive from explicit (TarInfo, payload) members.

    The suites use this to publish an archive that is wrong in one specific
    way — a missing executable, a symlinked one — without restating the whole
    tar/gzip boilerplate each time.
    """
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as package:
        for member, payload in members:
            package.addfile(member, io.BytesIO(payload))
    return buffer.getvalue()


def file_member(name, payload, mode=0o755):
    """A regular-file member carrying payload."""
    member = tarfile.TarInfo(name)
    member.size = len(payload)
    member.mode = mode
    return member, payload


def symlink_member(name, target):
    """A symlink member, which a release must never ship as its executable."""
    member = tarfile.TarInfo(name)
    member.type = tarfile.SYMTYPE
    member.linkname = target
    return member, b""


def script_payload(version):
    """An executable that reports one version, for observing a replace."""
    return f"#!/bin/sh\nprintf '%s\\n' '{version}'\n".encode()


class ReleaseServer:
    """Serves GitHub's release layout plus the API route the updater reads.

    Layout, mirroring GitHub: a tagged download lives under
    releases/download/<tag>/, and the newest release is aliased at
    releases/latest/download/. The updater additionally reads
    releases/latest for the current tag as JSON.
    """

    def __init__(self, root, platform):
        self.root = root
        self.releases = root / "releases"
        self.releases.mkdir(parents=True)
        # platform names the archive, e.g. "linux-x64".
        self.platform = platform
        self.latest_tag = ""
        self.instance = None

    def publish(self, tag, archive, make_latest=False, checksum="correct"):
        """Write a prebuilt archive for one tag, with its checksum sidecar.

        Callers encode once through archive_bytes (or their own cache) and pass
        the bytes, so a suite that publishes the same large payload repeatedly
        does not re-encode it per test.

        checksum selects which failure the fixture proves:
          "correct"  a digest that matches the archive;
          "wrong"    a well-formed digest for different bytes; and
          "garbage"  text that is not a digest at all.
        The two broken forms fail at different points, so a suite that needs one
        must not silently get the other.
        """
        directory = self.releases / "download" / tag
        directory.mkdir(parents=True, exist_ok=True)
        name = f"ompig-slim-{self.platform}.tar.gz"
        if checksum == "correct":
            digest = sha256_bytes(archive)
        elif checksum == "wrong":
            digest = "0" * 64
        elif checksum == "garbage":
            digest = "not a digest"
        else:
            raise ValueError(f"unknown checksum mode: {checksum}")
        checksum_line = f"{digest}  {name}\n"
        (directory / name).write_bytes(archive)
        pathlib.Path(str(directory / name) + ".sha256").write_text(checksum_line)
        if make_latest:
            self.latest_tag = tag
            latest = self.releases / "latest" / "download"
            latest.mkdir(parents=True, exist_ok=True)
            (latest / name).write_bytes(archive)
            pathlib.Path(str(latest / name) + ".sha256").write_text(checksum_line)
        return directory

    def start(self):
        """Serve the tree on loopback and return the release URL root."""
        server = self

        class Handler(http.server.SimpleHTTPRequestHandler):
            def do_GET(self):
                # The updater asks for the newest tag as JSON. The installer only
                # follows the latest/download alias, which the tree serves.
                if self.path.endswith("/releases/latest"):
                    body = json.dumps({"tag_name": server.latest_tag}).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                super().do_GET()

            def log_message(self, *args):
                pass

        self.instance = http.server.ThreadingHTTPServer(
            ("127.0.0.1", 0), lambda *a, **k: Handler(*a, directory=str(self.root), **k))
        threading.Thread(target=self.instance.serve_forever, daemon=True).start()
        host, port = self.instance.server_address
        return f"http://{host}:{port}"

    def stop(self):
        if self.instance is not None:
            self.instance.shutdown()
            self.instance.server_close()
