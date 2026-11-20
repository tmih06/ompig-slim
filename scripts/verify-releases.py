#!/usr/bin/env python3
"""Verify checksums, native executable headers, and complete bundled licenses."""
import hashlib
import pathlib
import stat
import struct
import sys
import tarfile
import zipfile

ARTIFACTS = (
    ("linux-x64", "tar.gz", 62),
    ("linux-arm64", "tar.gz", 183),
    ("darwin-x64", "tar.gz", 0x01000007),
    ("darwin-arm64", "tar.gz", 0x0100000C),
    ("windows-x64", "zip", 0x8664),
    ("windows-arm64", "zip", 0xAA64),
)


def verify_pe(binary, machine):
    assert len(binary) >= 0x40 and binary[:2] == b"MZ", "expected DOS header"
    pe_offset = struct.unpack_from("<I", binary, 0x3C)[0]
    assert pe_offset <= len(binary) - 24, "truncated PE header"
    assert binary[pe_offset:pe_offset + 4] == b"PE\0\0", "expected PE signature"
    actual_machine = struct.unpack_from("<H", binary, pe_offset + 4)[0]
    assert actual_machine == machine, f"wrong PE machine: {actual_machine:#x}"
    optional_size = struct.unpack_from("<H", binary, pe_offset + 20)[0]
    optional = pe_offset + 24
    assert optional_size >= 70 and optional + optional_size <= len(binary), "truncated PE optional header"
    assert struct.unpack_from("<H", binary, optional)[0] == 0x20B, "expected PE32+ executable"
    assert struct.unpack_from("<H", binary, optional + 68)[0] == 3, "expected console subsystem"


def safe_member_name(name, windows=False):
    parts = name.removesuffix("/").split("/")
    assert parts[0] == "ompig-slim" and all(part not in ("", ".", "..") for part in parts), name
    assert not any(character in name for character in ("\\", ":", "\0")), name
    if windows:
        assert not any(character in name for character in '<>"|?*') and all(ord(character) >= 32 for character in name), name
        devices = {"CON", "PRN", "AUX", "NUL"} | {
            prefix + digit for prefix in ("COM", "LPT") for digit in "123456789¹²³"
        }
        assert all(part == part.rstrip(". ") and part.split(".")[0].upper() not in devices for part in parts), name


def verify(directory):
    expected = {f"ompig-slim-{platform}.{extension}" for platform, extension, _ in ARTIFACTS}
    expected |= {name + ".sha256" for name in expected}
    actual = {path.name for path in directory.iterdir() if path.is_file() and
              path.name.endswith((".tar.gz", ".zip", ".sha256"))}
    assert actual == expected, f"unexpected release files: missing={expected - actual}, extra={actual - expected}"
    vendor = pathlib.Path(__file__).resolve().parent.parent / "vendor"
    pig = vendor / "github.com/MichaelKinsy/PiG"
    notices = [path for path in vendor.rglob("*") if path.is_file() and (
        any(path.name.upper().startswith(prefix) for prefix in
            ("LICENSE", "LICENCE", "COPYING", "COPYRIGHT", "NOTICE", "PATENTS"))
        or "NOTICES" in path.name.upper()
    )]
    for platform, extension, machine in ARTIFACTS:
        archive = directory / f"ompig-slim-{platform}.{extension}"
        digest, filename = pathlib.Path(str(archive) + ".sha256").read_text().split()
        assert filename == archive.name, f"wrong checksum filename: {filename}"
        assert hashlib.sha256(archive.read_bytes()).hexdigest() == digest, archive
        is_zip = extension == "zip"
        package = zipfile.ZipFile(archive) if is_zip else tarfile.open(archive, "r:gz")
        with package:
            members = package.infolist() if is_zip else package.getmembers()
            names = [member.filename if is_zip else member.name for member in members]
            keys = [name.removesuffix("/").casefold() if is_zip else name for name in names]
            assert len(keys) == len(set(keys)), "duplicate archive member"
            for member, name in zip(members, names):
                safe_member_name(name, windows=is_zip)
                if is_zip:
                    member_type = stat.S_IFMT(member.external_attr >> 16)
                    expected_types = (0, stat.S_IFDIR) if member.is_dir() else (0, stat.S_IFREG)
                    assert member_type in expected_types, f"unexpected member: {name}"
                else:
                    assert member.isfile() or member.isdir(), f"unexpected member: {name}"

            def content(name):
                member_name = "ompig-slim/" + name
                if is_zip:
                    return package.read(member_name)
                stream = package.extractfile(member_name)
                assert stream is not None, member_name
                with stream:
                    return stream.read()

            binary_name = "ompig-slim.exe" if is_zip else "ompig-slim"
            binary = content(binary_name)
            if is_zip:
                verify_pe(binary, machine)
            elif platform.startswith("linux"):
                assert binary[:6] == b"\x7fELF\x02\x01", "expected 64-bit little-endian ELF"
                assert struct.unpack_from("<H", binary, 18)[0] == machine
                assert package.getmember("ompig-slim/ompig-slim").mode & 0o111
            else:
                assert binary[:4] == b"\xcf\xfa\xed\xfe", "expected 64-bit little-endian Mach-O"
                assert struct.unpack_from("<I", binary, 4)[0] == machine
                assert package.getmember("ompig-slim/ompig-slim").mode & 0o111
            for name in ("README.md", "LICENSE", "NOTICE"):
                assert content(name) == (vendor.parent / name).read_bytes(), name
            for name in ("LICENSE", "NOTICE"):
                assert content("PiG-" + name) == (pig / name).read_bytes(), name
            for path in notices:
                assert content("licenses/" + path.relative_to(vendor).as_posix()) == path.read_bytes(), path
            for path in (pig / "LICENSES").iterdir():
                if path.is_file():
                    assert content("licenses/github.com/MichaelKinsy/PiG/LICENSES/" + path.name) == path.read_bytes(), path
            assert content("licenses/github.com/MichaelKinsy/PiG/REUSE.toml") == (pig / "REUSE.toml").read_bytes()
        print(f"{platform}: checksum, executable, and license contents verified")


if __name__ == "__main__":
    verify(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "bin/releases"))
