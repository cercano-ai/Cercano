"""Offline submission boundary tests. No live feed or credentials used."""
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

import publish_packages as publisher


class PublishTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.names = [f"{p}.1.2.3.nupkg" for p in publisher.pack_packages.PACKAGE_IDS]
        self.digests = {}
        for name in self.names:
            data = name.encode()
            (self.root / name).write_bytes(data)
            digest = hashlib.sha256(data).hexdigest()
            self.digests[name] = digest
            (self.root / (name + ".sha256")).write_text(f"{digest}  {name}\n")
        self.send = Mock()

    def publish(self, key="fixture-not-a-secret"):
        publisher.publish("1.2.3", None, self.root, self.digests, key, self.send)

    def test_order_exact_bytes_and_no_approval_claim(self):
        with contextlib.redirect_stdout(io.StringIO()) as output:
            self.publish()
        self.assertEqual([call.args[0] for call in self.send.call_args_list], self.names)
        for call in self.send.call_args_list:
            self.assertEqual(call.args[1], call.args[0].encode())
        self.assertIn("moderation approval is separate", output.getvalue())

    def test_any_error_stops_immediately_without_retry(self):
        for position in range(3):
            with self.subTest(position=position):
                self.send.reset_mock()
                self.send.side_effect = [None] * position + [RuntimeError("ambiguous")]
                with self.assertRaises(RuntimeError):
                    self.publish()
                self.assertEqual(self.send.call_count, position + 1)

    def test_all_checksums_verified_before_any_submission(self):
        for name in self.names:
            original = (self.root / name).read_bytes()
            (self.root / name).write_bytes(b"different run")
            with self.assertRaises(publisher.ValidationError):
                self.publish()
            self.send.assert_not_called()
            (self.root / name).write_bytes(original)
        (self.root / (self.names[-1] + ".sha256")).write_text("wrong sidecar")
        with self.assertRaises(publisher.ValidationError):
            self.publish()
        self.send.assert_not_called()

    def test_missing_key_wrong_manifest_and_extra_artifact_fail_closed(self):
        with self.assertRaises(publisher.ValidationError):
            self.publish("")
        self.digests.pop(self.names[-1])
        with self.assertRaises(publisher.ValidationError):
            self.publish()
        self.send.assert_not_called()

    def test_transport_only_201_no_redirect_retry_or_server_logging(self):
        for status in (200, 202, 204, 301, 302, 307, 400, 401, 403, 409, 429, 500, 503, 201):
            with self.subTest(status=status), patch.object(publisher.http.client, "HTTPSConnection") as factory:
                connection = factory.return_value
                connection.getresponse.return_value.status = status
                output = io.StringIO()
                with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
                    if status == 201:
                        publisher.submit(self.names[0], b"verified", "fixture-key")
                    else:
                        with self.assertRaises(publisher.ValidationError):
                            publisher.submit(self.names[0], b"verified", "fixture-key")
                connection.request.assert_called_once()
                connection.getresponse.assert_called_once()
                connection.getresponse.return_value.read.assert_not_called()
                connection.close.assert_called_once()
                request = connection.request.call_args
                self.assertEqual(request.args, ("PUT", "/api/v2/package"))
                self.assertEqual(request.kwargs["headers"]["X-NuGet-ApiKey"], "fixture-key")
                self.assertIn(b"verified", request.kwargs["body"])
                self.assertEqual(output.getvalue(), "")

    def test_http_failure_stops_remaining_packages_without_retry(self):
        for status in (409, 202, 500):
            for position in range(3):
                with self.subTest(status=status, position=position), \
                        patch.object(publisher.http.client, "HTTPSConnection") as factory, \
                        contextlib.redirect_stdout(io.StringIO()):
                    factory.return_value.getresponse.side_effect = [
                        Mock(status=code) for code in [201] * position + [status]]
                    with self.assertRaises(publisher.ValidationError):
                        publisher.publish("1.2.3", None, self.root, self.digests,
                                          "fixture-key", publisher.submit)
                    self.assertEqual(factory.return_value.request.call_count, position + 1)

    def test_submits_retained_verified_bytes_even_if_files_change(self):
        def send(name, data, key):
            for other in self.names:
                (self.root / other).write_bytes(b"replaced after validation")
            self.assertEqual(data, name.encode())
        with contextlib.redirect_stdout(io.StringIO()):
            publisher.publish("1.2.3", None, self.root, self.digests, "fixture-key", send)

    def test_timeout_is_ambiguous_and_not_retried(self):
        with patch.object(publisher.http.client, "HTTPSConnection") as factory:
            factory.return_value.getresponse.side_effect = TimeoutError("fixture-key")
            with self.assertRaisesRegex(publisher.ValidationError, "ambiguous") as error:
                publisher.submit(self.names[0], b"verified", "fixture-key")
            self.assertNotIn("fixture-key", str(error.exception))
            factory.return_value.request.assert_called_once()

    def test_main_sanitizes_arbitrary_failure_and_does_not_spawn_process(self):
        with patch.dict(os.environ, {"CHOCOLATEY_API_KEY": "fixture-key", "PACKAGE_DIGESTS": json.dumps(self.digests)}), \
                patch.object(publisher, "publish", side_effect=RuntimeError("fixture-key")), \
                contextlib.redirect_stderr(io.StringIO()) as output:
            self.assertEqual(publisher.main(["--version", "1.2.3", "--packages", str(self.root)]), 1)
            self.assertNotIn("fixture-key", output.getvalue())
            self.assertNotIn("CHOCOLATEY_API_KEY", os.environ)


if __name__ == "__main__":
    unittest.main()
