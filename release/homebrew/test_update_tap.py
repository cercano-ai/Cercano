import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

import update_tap as u


class PublicationGuardTest(unittest.TestCase):
    def test_split_cannot_publish_even_with_token(self):
        with patch.object(u, 'api') as api, patch.object(u, 'download') as download:
            with self.assertRaisesRegex(u.Failure, 'publication is disabled'):
                u.update('1.2.3', '0' * 64, Path('absent'), Path('absent'), 'fixture-token')
            api.assert_not_called()
            download.assert_not_called()


class TransportTest(unittest.TestCase):
    def test_api_auth_is_header_only_and_body_never_logged(self):
        error = urllib.error.HTTPError('https://api.github.com/path', 409,
                                      'sensitive-response-value', {}, None)
        with patch.object(u.urllib.request, 'build_opener') as opener:
            opener.return_value.open.side_effect = error
            with self.assertRaises(u.Failure) as caught:
                u.api('PUT', u.TAP_PATH, 'secret-fixture', {'branch': 'main'})
            self.assertEqual(caught.exception.status, 409)
            self.assertNotIn('secret-fixture', str(caught.exception))
            self.assertNotIn('sensitive-response-value', str(caught.exception))
            request = opener.return_value.open.call_args.args[0]
            self.assertEqual(request.full_url, 'https://api.github.com' + u.TAP_PATH)
            self.assertEqual(request.get_header('Authorization'), 'Bearer secret-fixture')
            self.assertNotIn(b'secret-fixture', request.data)
            self.assertEqual(request.get_header('Content-type'), 'application/json')

    def test_public_download_never_carries_credentials(self):
        body = b'public-archive'
        response = io.BytesIO(body)
        response.status = 200
        response.geturl = lambda: 'https://release-assets.githubusercontent.com/x'
        with patch.dict(u.os.environ, {'GITHUB_TOKEN': 'read-secret',
                                       'HOMEBREW_TAP_TOKEN': 'tap-secret'}):
            with patch.object(u.urllib.request, 'build_opener') as opener:
                opener.return_value.open.return_value = response
                self.assertEqual(u.download('https://github.com/x'), hashlib.sha256(body).hexdigest())
                self.assertEqual(opener.return_value.open.call_args.args, ('https://github.com/x',))

    def test_download_checks_final_redirect_destination(self):
        response = io.BytesIO(b'untrusted')
        response.status = 200
        response.geturl = lambda: 'https://evil.test/x'
        with patch.object(u.urllib.request, 'build_opener') as opener:
            opener.return_value.open.return_value = response
            with self.assertRaises(u.Failure):
                u.download('https://github.com/x')


if __name__ == '__main__':
    unittest.main()
