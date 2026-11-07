#!/usr/bin/env python3
"""Verify checksums, native executable headers, and complete bundled licenses."""
import hashlib
import pathlib
import struct
import sys
import tarfile


def verify(directory):
    vendor = pathlib.Path(__file__).resolve().parent.parent / "vendor"
    pig = vendor / "github.com/MichaelKinsy/PiG"
    notices = [path for path in vendor.rglob("*") if path.is_file() and (
        any(path.name.upper().startswith(prefix) for prefix in
            ("LICENSE", "LICENCE", "COPYING", "COPYRIGHT", "NOTICE", "PATENTS"))
        or "NOTICES" in path.name.upper()
    )]
    for platform, machine in (("linux-x64", 62), ("linux-arm64", 183),
                              ("darwin-x64", 0x01000007), ("darwin-arm64", 0x0100000C)):
        archive = directory / f"ompig-slim-{platform}.tar.gz"
        digest, filename = pathlib.Path(str(archive) + ".sha256").read_text().split()
        assert filename == archive.name, f"wrong checksum filename: {filename}"
        assert hashlib.sha256(archive.read_bytes()).hexdigest() == digest, archive
        with tarfile.open(archive, "r:gz") as package:
            members = package.getmembers()
            names = [member.name for member in members]
            assert len(names) == len(set(names)), "duplicate archive member"
            for member in members:
                path = pathlib.PurePosixPath(member.name)
                assert path.parts[0] == "ompig-slim" and ".." not in path.parts, path
                assert member.isfile() or member.isdir(), f"unexpected member: {member.name}"
            def content(name):
                return package.extractfile("ompig-slim/" + name).read()
            binary = content("ompig-slim")
            assert package.getmember("ompig-slim/ompig-slim").mode & 0o111
            if platform.startswith("linux"):
                assert binary[:6] == b"\x7fELF\x02\x01", "expected 64-bit little-endian ELF"
                assert struct.unpack_from("<H", binary, 18)[0] == machine
            else:
                assert binary[:4] == b"\xcf\xfa\xed\xfe", "expected 64-bit little-endian Mach-O"
                assert struct.unpack_from("<I", binary, 4)[0] == machine
            for name in ("README.md", "LICENSE", "NOTICE"):
                assert content(name) == (vendor.parent / name).read_bytes(), name
            for name in ("LICENSE", "NOTICE"):
                assert content("PiG-" + name) == (pig / name).read_bytes(), name
            for path in notices:
                assert content("licenses/" + str(path.relative_to(vendor))) == path.read_bytes(), path
            for path in (pig / "LICENSES").iterdir():
                if path.is_file():
                    assert content("licenses/github.com/MichaelKinsy/PiG/LICENSES/" + path.name) == path.read_bytes(), path
            assert content("licenses/github.com/MichaelKinsy/PiG/REUSE.toml") == (pig / "REUSE.toml").read_bytes()
        print(f"{platform}: checksum, executable, and license contents verified")


if __name__ == "__main__":
    verify(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "bin/releases"))
