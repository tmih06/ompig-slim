"""Check the loopback Windows release fixture without requiring Windows."""
import http.client
import importlib.util
import io
from pathlib import Path
import unittest
import zipfile

SPEC = importlib.util.spec_from_file_location(
    "windows_install", Path(__file__).with_name("test-install-windows.py"))
windows_install = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(windows_install)


class WindowsReleaseFixtureTests(unittest.TestCase):
    def test_declared_size_edit_ignores_executable_zip_signatures(self):
        data = windows_install.build_archive(executable=b"executable PK\x01\x02" + b"\xff" * 100,
                                             declared_size=123456789)
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            self.assertEqual(archive.getinfo(windows_install.ARCHIVE_MEMBER).file_size, 123456789)

    def test_latest_redirect_quotes_location_components(self):
        server = windows_install.ReleaseServer("windows-x64")
        self.addCleanup(server.close)
        server.publish("v1.0.0\r\nInjected: value", b"fixture", latest=True)
        connection = http.client.HTTPConnection(*server.server.server_address, timeout=5)
        self.addCleanup(connection.close)
        connection.request("GET", "/releases/latest/download/file%0D%0A.zip")
        response = connection.getresponse()
        self.assertEqual(response.status, 302)
        self.assertEqual(response.getheader("Location"),
                         "/releases/download/v1.0.0%0D%0AInjected%3A%20value/file%250D%250A.zip")
        self.assertIsNone(response.getheader("Injected"))

    def test_latest_redirect_preserves_release_asset_path(self):
        server = windows_install.ReleaseServer("windows-arm64")
        self.addCleanup(server.close)
        server.publish("v1.0.0", b"fixture", latest=True)
        connection = http.client.HTTPConnection(*server.server.server_address, timeout=5)
        self.addCleanup(connection.close)
        connection.request("GET", "/releases/latest/download/ompig-slim-windows-arm64.zip")
        response = connection.getresponse()
        self.assertEqual(response.status, 302)
        self.assertEqual(response.getheader("Location"),
                         "/releases/download/v1.0.0/ompig-slim-windows-arm64.zip")


if __name__ == "__main__":
    unittest.main()
