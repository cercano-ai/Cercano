"""No-network tests of trusted digest, bounded download and redirect policy."""
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

import download_release as d


class DownloadTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.directory = Path(self.tmp.name) / "downloads"
        self.body = b"fixture ZIP bytes"
        self.digest = hashlib.sha256(self.body).hexdigest()

    def fetch(self, body=None, url="https://release-assets.githubusercontent.com/asset"):
        response = MagicMock()
        response.__enter__.return_value = response
        response.geturl.return_value = url
        response.status = 200
        response.read1.side_effect = io.BytesIO(self.body if body is None else body).read
        opener = MagicMock()
        opener.open.return_value = response
        return patch.object(d.urllib.request, "build_opener", return_value=opener)

    def test_download_matches_independent_digest_then_commits_atomically(self):
        with self.fetch() as opener:
            path = d.download("1.2.3", self.digest.upper(), self.directory)
        self.assertEqual(path.read_bytes(), self.body)
        self.assertEqual(list(self.directory.iterdir()), [path])
        self.assertEqual(opener.return_value.open.call_args.args, (
            "https://github.com/cercano-ai/Cercano/releases/download/v1.2.3/cercano-1.2.3-windows-x64.zip",))
        self.assertEqual(opener.return_value.open.call_args.kwargs, {"timeout": 30})

    def test_bad_inputs_never_connect(self):
        with patch.object(d.urllib.request, "build_opener") as opener:
            for version, digest in [("$(touch bad)", self.digest), ("01.2.3", self.digest),
                                    ("1.2.3", "x" * 64), ("1.2.3", self.digest + "\n")]:
                with self.assertRaises(d.render_packages.ValidationError):
                    d.download(version, digest, self.directory)
            opener.assert_not_called()
        self.assertFalse(self.directory.exists())

    def test_checksum_failure_leaves_no_archive(self):
        with self.fetch(), self.assertRaisesRegex(d.Failure, "sha256 mismatch"):
            d.download("1.2.3", "0" * 64, self.directory)
        self.assertEqual(list(self.directory.iterdir()), [])

    def test_bounds_and_final_redirect_checked(self):
        with self.fetch(), patch.object(d, "MAX_ARCHIVE", 1), self.assertRaisesRegex(d.Failure, "limit"):
            d.download("1.2.3", self.digest, self.directory)
        with self.fetch(), patch.object(d.time, "monotonic", side_effect=[0, 301]), self.assertRaisesRegex(d.Failure, "limit"):
            d.download("1.2.3", self.digest, self.directory)
        with self.fetch(url="https://evil.example/asset"), self.assertRaises(d.Failure):
            d.download("1.2.3", self.digest, self.directory)
        self.assertEqual(list(self.directory.iterdir()), [])

    def test_each_redirect_is_https_github_only(self):
        redirect = d.PublicRedirect()
        for url in ("http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x",
                    "https://user:password@github.com/x", "https://github.com:444/x"):
            with self.subTest(url=url), self.assertRaises(d.Failure):
                redirect.redirect_request(None, None, 302, "Found", {}, url)


if __name__ == "__main__":
    unittest.main()
