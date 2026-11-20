"""Regression tests for the native release archive verifier."""
import contextlib
import hashlib
import importlib.util
import io
import pathlib
import shutil
import stat
import struct
import tarfile
import tempfile
import unittest
import zipfile

SCRIPTS = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("verify_releases", SCRIPTS / "verify-releases.py")
verify_releases = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(verify_releases)

ROOT = SCRIPTS.parent
VENDOR = ROOT / "vendor"
PIG = VENDOR / "github.com/MichaelKinsy/PiG"
ARTIFACTS = (
    ("linux-x64", "tar.gz", 62),
    ("linux-arm64", "tar.gz", 183),
    ("darwin-x64", "tar.gz", 0x01000007),
    ("darwin-arm64", "tar.gz", 0x0100000C),
    ("windows-x64", "zip", 0x8664),
    ("windows-arm64", "zip", 0xAA64),
)


def pe_image(machine, magic=0x20B, subsystem=3):
    pe_offset = 0x80
    image = bytearray(pe_offset + 24 + 70)
    image[:2] = b"MZ"
    struct.pack_into("<I", image, 0x3C, pe_offset)
    image[pe_offset:pe_offset + 4] = b"PE\0\0"
    struct.pack_into("<H", image, pe_offset + 4, machine)
    struct.pack_into("<H", image, pe_offset + 20, 70)
    optional = pe_offset + 24
    struct.pack_into("<H", image, optional, magic)
    struct.pack_into("<H", image, optional + 68, subsystem)
    return bytes(image)


def executable(platform, machine):
    if platform.startswith("windows"):
        return pe_image(machine)
    if platform.startswith("linux"):
        image = bytearray(64)
        image[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", image, 18, machine)
        return bytes(image)
    image = bytearray(8)
    image[:4] = b"\xcf\xfa\xed\xfe"
    struct.pack_into("<I", image, 4, machine)
    return bytes(image)


def package_files(platform, machine):
    binary_name = "ompig-slim.exe" if platform.startswith("windows") else "ompig-slim"
    files = {
        "README.md": (ROOT / "README.md").read_bytes(),
        "LICENSE": (ROOT / "LICENSE").read_bytes(),
        "NOTICE": (ROOT / "NOTICE").read_bytes(),
        "PiG-LICENSE": (PIG / "LICENSE").read_bytes(),
        "PiG-NOTICE": (PIG / "NOTICE").read_bytes(),
        binary_name: executable(platform, machine),
        "licenses/github.com/MichaelKinsy/PiG/REUSE.toml": (PIG / "REUSE.toml").read_bytes(),
    }
    for path in VENDOR.rglob("*"):
        if path.is_file() and (
            any(path.name.upper().startswith(prefix) for prefix in
                ("LICENSE", "LICENCE", "COPYING", "COPYRIGHT", "NOTICE", "PATENTS"))
            or "NOTICES" in path.name.upper()
        ):
            files["licenses/" + path.relative_to(VENDOR).as_posix()] = path.read_bytes()
    for path in (PIG / "LICENSES").rglob("*"):
        if path.is_file():
            relative = path.relative_to(PIG / "LICENSES").as_posix()
            files[f"licenses/github.com/MichaelKinsy/PiG/LICENSES/{relative}"] = path.read_bytes()
    return files


def write_archive(directory, platform, files, extras=()):
    extension = next(ext for name, ext, _ in ARTIFACTS if name == platform)
    archive = directory / f"ompig-slim-{platform}.{extension}"
    binary_name = "ompig-slim.exe" if extension == "zip" else "ompig-slim"
    if extension == "zip":
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as package:
            for name, payload in sorted(files.items()):
                info = zipfile.ZipInfo("ompig-slim/" + name)
                info.create_system = 3
                mode = stat.S_IFREG | (0o755 if name == binary_name else 0o644)
                info.external_attr = mode << 16
                package.writestr(info, payload)
            for name, payload, kind in extras:
                info = zipfile.ZipInfo(name)
                info.create_system = 3
                member_type = stat.S_IFLNK if kind == "symlink" else stat.S_IFREG
                info.external_attr = (member_type | 0o777) << 16
                package.writestr(info, payload)
    else:
        with tarfile.open(archive, "w:gz") as package:
            for name, payload in sorted(files.items()):
                info = tarfile.TarInfo("ompig-slim/" + name)
                info.size = len(payload)
                info.mode = 0o755 if name == binary_name else 0o644
                package.addfile(info, io.BytesIO(payload))
            for name, payload, kind in extras:
                info = tarfile.TarInfo(name)
                if kind == "symlink":
                    info.type = tarfile.SYMTYPE
                    info.linkname = payload.decode()
                    package.addfile(info)
                else:
                    info.size = len(payload)
                    package.addfile(info, io.BytesIO(payload))
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    pathlib.Path(str(archive) + ".sha256").write_text(f"{digest}  {archive.name}\n")
    return archive


class ReleaseVerifierTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.templates = tempfile.TemporaryDirectory()
        cls.template_dir = pathlib.Path(cls.templates.name)
        cls.files = {}
        for platform, _, machine in ARTIFACTS:
            cls.files[platform] = package_files(platform, machine)
            write_archive(cls.template_dir, platform, cls.files[platform])

    @classmethod
    def tearDownClass(cls):
        cls.templates.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.releases = pathlib.Path(self.temporary.name)
        shutil.copytree(self.template_dir, self.releases, dirs_exist_ok=True)

    def tearDown(self):
        self.temporary.cleanup()

    def verify_silently(self):
        with contextlib.redirect_stdout(io.StringIO()):
            verify_releases.verify(self.releases)

    def rewrite(self, platform, extras=(), files=None):
        return write_archive(self.releases, platform, files or self.files[platform], extras)

    def test_verifies_all_six_archives_and_full_notices(self):
        with contextlib.redirect_stdout(io.StringIO()) as output:
            verify_releases.verify(self.releases)
        self.assertEqual(
            [line.split(":", 1)[0] for line in output.getvalue().splitlines()],
            [platform for platform, _, _ in ARTIFACTS],
        )
        self.assertEqual(
            {path.name for path in self.releases.glob("ompig-slim-*.*") if not path.name.endswith(".sha256")},
            {f"ompig-slim-{platform}.{extension}" for platform, extension, _ in ARTIFACTS},
        )

    def test_rejects_bad_pe_machine_magic_and_subsystem(self):
        for machine, magic, subsystem in (
            (0xAA64, 0x20B, 3),
            (0x8664, 0x10B, 3),
            (0x8664, 0x20B, 2),
        ):
            with self.subTest(machine=machine, magic=magic, subsystem=subsystem):
                files = dict(self.files["windows-x64"])
                files["ompig-slim.exe"] = pe_image(machine, magic, subsystem)
                self.rewrite("windows-x64", files=files)
                with self.assertRaises(AssertionError):
                    self.verify_silently()

    def test_rejects_unsafe_zip_members(self):
        for extra in (
            ("ompig-slim/../outside", b"escape", "regular"),
            ("ompig-slim/link", b"target", "symlink"),
            ("ompig-slim/OMPIG-SLIM.EXE", b"alias", "regular"),
            ("ompig-slim/stream:payload", b"alias", "regular"),
            ("ompig-slim/trailing.", b"alias", "regular"),
            ("ompig-slim/trailing ", b"alias", "regular"),
            ("ompig-slim/NUL.txt", b"alias", "regular"),
        ):
            with self.subTest(member=extra[0]):
                self.rewrite("windows-x64", extras=(extra,))
                with self.assertRaises(AssertionError):
                    self.verify_silently()

    def test_rejects_unsafe_tar_members(self):
        self.rewrite("linux-x64", extras=(("ompig-slim/link", b"target", "symlink"),))
        with self.assertRaises(AssertionError):
            self.verify_silently()

    def test_rejects_unverified_extra_release_assets(self):
        for name in ("unverified.zip", "unverified.tar.gz", "unverified.sha256"):
            with self.subTest(name=name):
                extra = self.releases / name
                extra.write_bytes(b"not verified")
                try:
                    with self.assertRaises(AssertionError):
                        self.verify_silently()
                finally:
                    extra.unlink()

    def test_rejects_malformed_zip(self):
        archive = self.releases / "ompig-slim-windows-x64.zip"
        archive.write_bytes(b"not a zip archive")
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        pathlib.Path(str(archive) + ".sha256").write_text(f"{digest}  {archive.name}\n")
        with self.assertRaises(zipfile.BadZipFile):
            self.verify_silently()

    def test_rejects_checksum_mismatch(self):
        archive = self.releases / "ompig-slim-windows-arm64.zip"
        archive.write_bytes(archive.read_bytes() + b"changed")
        with self.assertRaises(AssertionError):
            self.verify_silently()

    def test_rejects_changed_vendor_notice(self):
        notice = next(
            path for path in VENDOR.rglob("*")
            if path.is_file() and path.name.upper().startswith("LICENSE")
        )
        member = "licenses/" + notice.relative_to(VENDOR).as_posix()
        files = dict(self.files["windows-x64"])
        files[member] = b"changed notice\n"
        self.rewrite("windows-x64", files=files)
        with self.assertRaises(AssertionError):
            self.verify_silently()


if __name__ == "__main__":
    unittest.main()
