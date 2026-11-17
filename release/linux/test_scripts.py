"""Hermetic shell failure propagation; never starts Docker or accesses keys."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent


class MatrixTests(unittest.TestCase):
    def run_matrix(self, fail_at, upgrade=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            docker = root / 'docker'
            docker.write_text('''#!/bin/bash
n=$(cat "$COUNT" 2>/dev/null || echo 0)
n=$((n+1)); echo "$n" > "$COUNT"
[ "$n" != "$FAIL_AT" ] || exit 42
''')
            docker.chmod(0o755)
            env = dict(os.environ, PATH=tmp + ':' + os.environ['PATH'],
                       COUNT=str(root / 'count'), FAIL_AT=str(fail_at))
            args = ['bash', str(HERE / 'tests/run-container-matrix.sh'), tmp]
            if upgrade:
                args += [tmp, '1.2.3']
            result = subprocess.run(args, env=env, capture_output=True)
            return result.returncode, int((root / 'count').read_text())

    def test_lifecycle_failure_is_not_masked(self):
        self.assertEqual(self.run_matrix(1), (42, 1))

    def test_upgrade_failure_is_not_masked(self):
        self.assertEqual(self.run_matrix(2, True), (42, 2))

    def test_success_runs_entire_matrix(self):
        self.assertEqual(self.run_matrix(0), (0, 7))
        self.assertEqual(self.run_matrix(0, True), (0, 14))


class PackagingBoundaryTests(unittest.TestCase):
    def test_bad_archive_never_runs_packager_and_cleans_staging(self):
        from test_render_packages import write_archive, VERSION
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive, digest = write_archive(root)
            tool = root / 'nfpm'
            tool.write_text('#!/bin/sh\ntouch "$NFPM_MARKER"\nexit 99\n')
            tool.chmod(0o755)
            output = root / 'packages'
            env = dict(os.environ, PATH=tmp + ':' + os.environ['PATH'],
                       NFPM_MARKER=str(root / 'ran'))
            result = subprocess.run(['bash', str(HERE / 'package-verified.sh'),
                                     VERSION, str(archive), '0' * 64, str(output)],
                                    env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((root / 'ran').exists())
            self.assertFalse(output.exists())
            self.assertEqual(list(root.glob('.linux-packages.*')), [])
            # A verified render followed by a packager failure also cleans up.
            result = subprocess.run(['bash', str(HERE / 'package-verified.sh'),
                                     VERSION, str(archive), digest, str(output)],
                                    env=env, capture_output=True)
            self.assertEqual(result.returncode, 99, result.stderr)
            self.assertTrue((root / 'ran').exists())
            self.assertFalse(output.exists())
            self.assertEqual(list(root.glob('.linux-packages.*')), [])

    def test_installer_rejects_unverified_download_before_extraction(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name, body in {
                'uname': 'case "$1" in -s) echo Linux;; -m) echo x86_64;; esac',
                'curl': 'while [ "$1" != -o ]; do shift; done; printf bad > "$2"',
                'sha256sum': 'exit 1',
                'tar': 'touch "$TAR_MARKER"; exit 99',
            }.items():
                tool = root / name
                tool.write_text('#!/bin/sh\n' + body + '\n')
                tool.chmod(0o755)
            env = dict(os.environ, PATH=tmp + ':' + os.environ['PATH'],
                       TAR_MARKER=str(root / 'extracted'))
            result = subprocess.run(['bash', str(HERE / 'install-nfpm.sh'),
                                     str(root / 'tools')], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((root / 'extracted').exists())
            self.assertFalse((root / 'tools/nfpm').exists())


class SigningIsolationTests(unittest.TestCase):
    def test_relative_protected_passphrase_survives_site_chdir(self):
        # Inert fixtures only: exercise the public isolation-wrapper interface.
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            tools = root / 'bin'
            tools.mkdir()
            for name, body in {
                'gpg': """printf '%s' "$GNUPGHOME" > "$HOME_RECORD"
case "$*" in
  *--list-secret-keys*) printf 'sec::::::::::::::#:\n' ;;
esac
while [ "$#" -gt 0 ]; do
  if [ "$1" = --passphrase-file ]; then
    shift
    printf '%s\n' "$1" >> "$PATH_RECORD"
    [ -f "$1" ] && [ -r "$1" ] || exit 43
  fi
  shift
done
""",
                'gpgconf': 'exit 0',
                'stat': 'echo 700',  # GNU stat interface, including on macOS.
                'dpkg-deb': 'echo cercano-agent',
                'apt-ftparchive': "echo 'Package: cercano-agent'",
            }.items():
                tool = tools / name
                tool.write_text('#!/bin/bash\n' + body + '\n')
                tool.chmod(0o755)
            (root / 'debs').mkdir()
            (root / 'debs/test.deb').write_bytes(b'inert package')
            (root / 'protected file').write_text('NOT A SECRET: inert fixture\n')
            (root / 'protected file').chmod(0o600)
            env = dict(os.environ, PATH=str(tools) + ':' + os.environ['PATH'],
                       HOME_RECORD=str(root / 'home'), PATH_RECORD=str(root / 'paths'),
                       CERCANO_GPG_PASSPHRASE_FILE='protected file')
            result = subprocess.run(['bash', str(HERE / 'sign-apt-repo.sh'),
                                     'site', 'debs', 'inert-subkey', 'A' * 40],
                                    cwd=root, env=env, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((root / 'paths').read_text().splitlines(),
                             [str((root / 'protected file').resolve())] * 2)
            self.assertFalse(Path((root / 'home').read_text()).exists())
            for invalid in ('missing-file', 'debs'):
                with self.subTest(invalid=invalid):
                    env['CERCANO_GPG_PASSPHRASE_FILE'] = invalid
                    result = subprocess.run(
                        ['bash', str(HERE / 'sign-apt-repo.sh'),
                         'invalid-site', 'debs', 'inert-subkey', 'A' * 40],
                        cwd=root, env=env, capture_output=True)
                    self.assertEqual(result.returncode, 2, result.stderr)
                    self.assertIn(b'readable regular file', result.stderr)
                    self.assertFalse((root / 'invalid-site').exists())
                    self.assertFalse(Path((root / 'home').read_text()).exists())

    def test_import_failure_and_primary_rejection_clean_up(self):
        for mode, expected in (("import-fail", 31), ("primary", 2)):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                tools = root / "bin"
                tools.mkdir()
                gpg = tools / "gpg"
                gpg.write_text("""#!/bin/bash
printf '%s' "$GNUPGHOME" > "$HOME_RECORD"
case "$*" in
  *--import*) [ "$MODE" != import-fail ] || exit 31 ;;
  *--list-secret-keys*) printf 'sec::::::::::::::+:\n' ;;
esac
""")
                gpg.chmod(0o755)
                conf = tools / "gpgconf"
                conf.write_text("#!/bin/sh\nexit 0\n")
                conf.chmod(0o755)
                env = dict(os.environ, PATH=str(tools) + ':' + os.environ['PATH'],
                           HOME_RECORD=str(root / 'record'), MODE=mode,
                           GNUPGHOME=str(root / 'must-not-use'))
                result = subprocess.run(['bash', str(HERE / 'sign-apt-repo.sh'),
                                         str(root / 'site'), tmp, 'dummy-input', 'A' * 40],
                                        env=env, capture_output=True)
                self.assertEqual(result.returncode, expected, result.stderr)
                home = Path((root / 'record').read_text())
                self.assertNotEqual(home, root / 'must-not-use')
                self.assertFalse(home.exists(), "temporary signing home leaked")
                self.assertFalse((root / 'site').exists())


class AptTamperTests(unittest.TestCase):
    def test_rehearsal_mutates_signed_body_and_preserves_armor(self):
        script = (HERE / 'tests/rehearse-apt.sh').read_text()
        mutation = script.split('# BEGIN signed-body mutation', 1)[1].split('\n', 1)[1]
        mutation = mutation.split('# END signed-body mutation', 1)[0]
        self.assertIn("if apt_update; then echo 'ERROR: accepted tampered signature'", script)
        header = '-----BEGIN PGP SIGNED MESSAGE-----\nHash: SHA256\n\n'
        armor = '-----BEGIN PGP SIGNATURE-----\n\nOrigin: Cercano\ninert fixture\n-----END PGP SIGNATURE-----\n'
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / 'site/dists/stable/InRelease'
            target.parent.mkdir(parents=True)
            for origin, expected in (('Cercano', 0), ('Other', 1)):
                with self.subTest(origin=origin):
                    original = header + f'Origin: {origin}\nSuite: stable\n' + armor
                    target.write_text(original)
                    result = subprocess.run(['bash', '-eu', '-c', mutation],
                                            env=dict(os.environ, tmp=tmp), capture_output=True)
                    self.assertEqual(result.returncode, expected, result.stderr)
                    if expected:
                        self.assertEqual(target.read_text(), original)
                    else:
                        changed = target.read_text()
                        self.assertNotEqual(changed, original)
                        self.assertEqual(changed, header + 'Origin: Tampered\nSuite: stable\n' + armor)


class WorkflowBoundaryTests(unittest.TestCase):
    def test_linux_jobs_remain_secret_free_and_review_only(self):
        repo = HERE.parents[1]
        workflow = (repo / '.github/workflows/release-macos.yml').read_text()
        # Top-level jobs use two spaces; nested job fields use four or more.
        import re
        linux = re.split(r'\n  [A-Za-z][A-Za-z0-9_-]*:',
                         workflow.split('\n  build-linux:', 1)[1], maxsplit=1)[0]
        ci = (repo / '.github/workflows/ci.yml').read_text().split('\n  linux-packaging:', 1)[1]
        for job in (linux, ci):
            self.assertNotIn('secrets.', job)
            self.assertNotRegex(job, r'(?m)^    environment:')
            self.assertNotIn('deploy-pages', job)
            self.assertNotIn('gh release', job)
            self.assertIn('release/linux/install-nfpm.sh', job)
        self.assertIn('release/linux/package-verified.sh', linux)
        self.assertIn('steps.build.outputs.sha256', linux)
        self.assertIn('linux-packages-review', linux)
        self.assertIn('contents: read', ci)
        self.assertIn('release/linux/tests/rehearse-apt.sh', ci)


if __name__ == '__main__':
    unittest.main()
