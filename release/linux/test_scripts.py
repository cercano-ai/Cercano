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

    def test_arm64_packages_components_only(self):
        from test_render_packages import write_archive, default_files, fake_elf, VERSION
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            files = default_files()
            files['bin/cercano'] = (fake_elf('agent', machine=183), 0o755)
            files['bin/cercano-cli'] = (fake_elf('cli', machine=183), 0o755)
            archive, digest = write_archive(root, files=files, platform='linux-arm64')
            tool = root / 'nfpm'
            tool.write_text('#!/bin/bash\n'
                            'while [ $# -gt 0 ]; do case "$1" in -f) f=$2;; -p) p=$2;; -t) t=$2;; esac; shift; done\n'
                            'echo "$(basename "$f" .yaml) $p" >> "$NFPM_LOG"\n'
                            'grep -q "^arch: arm64$" "$f" || exit 98\n'
                            'touch "$t/$(basename "$f" .yaml).$p"\n')
            tool.chmod(0o755)
            env = dict(os.environ, PATH=tmp + ':' + os.environ['PATH'], NFPM_LOG=str(root / 'log'))
            output = root / 'packages'
            result = subprocess.run(['bash', str(HERE / 'package-verified.sh'),
                                     VERSION, str(archive), digest, str(output), 'arm64'],
                                    env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            built = (root / 'log').read_text().split('\n')
            self.assertEqual(sorted(line.split()[0] for line in built if line),
                             ['cercano-agent'] * 4 + ['cercano-cli'] * 4)
            self.assertEqual(sorted(p.name for p in output.iterdir()), sorted(
                ['SHA256SUMS'] + [f'{pkg}.{fmt}' for pkg in ('cercano-agent', 'cercano-cli')
                                  for fmt in ('deb', 'rpm', 'apk', 'archlinux')]))

    def test_unknown_arch_is_refused_before_rendering(self):
        from test_render_packages import write_archive, VERSION
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive, digest = write_archive(root)
            output = root / 'packages'
            for arch in ('x86_64', 'aarch64', ''):
                with self.subTest(arch=arch):
                    result = subprocess.run(['bash', str(HERE / 'package-verified.sh'),
                                             VERSION, str(archive), digest, str(output), arch],
                                            capture_output=True, text=True)
                    self.assertEqual(result.returncode, 2)
                    self.assertFalse(output.exists())
                    self.assertEqual(list(root.glob('.linux-packages.*')), [])

    def installer_fixture(self, root, machine):
        for name, body in {
            'uname': f'case "$1" in -s) echo Linux;; -m) echo {machine};; esac',
            'curl': 'echo "$@" >> "$CURL_LOG"; while [ "$1" != -o ]; do shift; done; printf bad > "$2"',
            'sha256sum': 'cat >> "$SHA_LOG"; exit 1',
            'tar': 'touch "$TAR_MARKER"; exit 99',
        }.items():
            tool = root / name
            tool.write_text('#!/bin/sh\n' + body + '\n')
            tool.chmod(0o755)
        env = dict(os.environ, PATH=str(root) + ':' + os.environ['PATH'],
                   TAR_MARKER=str(root / 'extracted'), CURL_LOG=str(root / 'curl.log'),
                   SHA_LOG=str(root / 'sha.log'))
        return subprocess.run(['bash', str(HERE / 'install-nfpm.sh'), str(root / 'tools')],
                              env=env, capture_output=True, text=True)

    def test_installer_pins_the_asset_for_each_architecture(self):
        for machine, asset, sha in (
                ('x86_64', 'nfpm_2.48.0_Linux_x86_64.tar.gz', '6d7f9a28b2f39793'),
                ('aarch64', 'nfpm_2.48.0_Linux_arm64.tar.gz', 'a9a75213763783b7')):
            with self.subTest(machine=machine), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                result = self.installer_fixture(root, machine)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('/v2.48.0/' + asset + ' ', (root / 'curl.log').read_text())
                self.assertTrue((root / 'sha.log').read_text().startswith(sha))
                self.assertFalse((root / 'extracted').exists())

    def test_installer_refuses_other_machines_before_download(self):
        for machine in ('armv7l', 'i686', 'riscv64'):
            with self.subTest(machine=machine), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                result = self.installer_fixture(root, machine)
                self.assertEqual(result.returncode, 2)
                self.assertFalse((root / 'curl.log').exists())

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


class MergeReviewTests(unittest.TestCase):
    AMD64 = ('cercano-agent_1.2.3_amd64.deb', 'cercano-cli_1.2.3_amd64.deb', 'cercano_1.2.3_all.deb',
             'cercano-agent-1.2.3-1.x86_64.rpm', 'cercano-1.2.3-1.noarch.rpm')
    ARM64 = ('cercano-agent_1.2.3_arm64.deb', 'cercano-cli_1.2.3_arm64.deb',
             'cercano-agent-1.2.3-1.aarch64.rpm', 'cercano-cli-1.2.3-1-aarch64.pkg.tar.zst')

    def make_set(self, root, label, names):
        directory = root / label
        directory.mkdir()
        for name in names:
            (directory / name).write_bytes(f'{label} {name}'.encode())
        subprocess.run('sha256sum ./* > SHA256SUMS', shell=True, cwd=directory, check=True)
        return directory

    def merge(self, root, amd64, arm64):
        return subprocess.run(['bash', str(HERE / 'merge-review-packages.sh'),
                               str(amd64), str(arm64), str(root / 'review')],
                              capture_output=True, text=True)

    def assert_refused(self, root, result, message):
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(message, result.stderr)
        self.assertFalse((root / 'review').exists())
        self.assertEqual(list(root.glob('.linux-review.*')), [])

    def test_merges_both_sets_with_one_inventory(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            amd64 = self.make_set(root, 'amd64', self.AMD64)
            arm64 = self.make_set(root, 'arm64', self.ARM64)
            result = self.merge(root, amd64, arm64)
            self.assertEqual(result.returncode, 0, result.stderr)
            review = root / 'review'
            self.assertEqual(sorted(p.name for p in review.iterdir()),
                             sorted([*self.AMD64, *self.ARM64, 'SHA256SUMS']))
            self.assertEqual((review / 'cercano_1.2.3_all.deb').read_bytes(), b'amd64 cercano_1.2.3_all.deb')
            lines = (review / 'SHA256SUMS').read_text().splitlines()
            self.assertEqual(sorted(line.split('  ./')[1] for line in lines), sorted([*self.AMD64, *self.ARM64]))
            subprocess.run(['sha256sum', '--strict', '-c', 'SHA256SUMS'], cwd=review, check=True,
                           capture_output=True)

    def test_refuses_architecture_independent_package_from_arm64(self):
        for extra in ('cercano_1.2.3_all.deb', 'cercano-1.2.3-1.noarch.rpm', 'other_1.2.3_arm64.deb'):
            with self.subTest(extra=extra), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                amd64 = self.make_set(root, 'amd64', self.AMD64)
                arm64 = self.make_set(root, 'arm64', (*self.ARM64, extra))
                self.assert_refused(root, self.merge(root, amd64, arm64),
                                    'only cercano-agent and cercano-cli')

    def test_refuses_name_in_both_sets(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            amd64 = self.make_set(root, 'amd64', (*self.AMD64, self.ARM64[0]))
            arm64 = self.make_set(root, 'arm64', self.ARM64)
            self.assert_refused(root, self.merge(root, amd64, arm64), 'both sets contain')

    def test_refuses_changed_unlisted_or_missing_files(self):
        cases = {
            'checksum mismatch': lambda d: (d / self.ARM64[0]).write_bytes(b'changed'),
            'does not list exactly': lambda d: (d / 'cercano-cli_9.9.9_arm64.deb').write_bytes(b'x'),
            'missing SHA256SUMS': lambda d: (d / 'SHA256SUMS').unlink(),
            'malformed SHA256SUMS': lambda d: (d / 'SHA256SUMS').write_text('nonsense\n'),
            'not a regular file': lambda d: (d / 'sub').mkdir(),
        }
        for message, damage in cases.items():
            with self.subTest(message=message), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                amd64 = self.make_set(root, 'amd64', self.AMD64)
                arm64 = self.make_set(root, 'arm64', self.ARM64)
                damage(arm64)
                self.assert_refused(root, self.merge(root, amd64, arm64), message)

    def test_refuses_existing_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            amd64 = self.make_set(root, 'amd64', self.AMD64)
            arm64 = self.make_set(root, 'arm64', self.ARM64)
            (root / 'review').mkdir()
            result = self.merge(root, amd64, arm64)
            self.assertEqual(result.returncode, 2)
            self.assertEqual(list((root / 'review').iterdir()), [])


class WorkflowBoundaryTests(unittest.TestCase):
    def test_linux_jobs_remain_secret_free_and_review_only(self):
        repo = HERE.parents[1]
        workflow = (repo / '.github/workflows/release-macos.yml').read_text()
        # Top-level jobs use two spaces; nested job fields use four or more.
        import re

        def job(name):
            return re.split(r'\n  [A-Za-z][A-Za-z0-9_-]*:',
                            workflow.split(f'\n  {name}:', 1)[1], maxsplit=1)[0]
        builders = {arch: job(name) for arch, name in (('amd64', 'build-linux'), ('arm64', 'build-linux-arm64'))}
        review = job('linux-packages-review')
        ci = (repo / '.github/workflows/ci.yml').read_text().split('\n  linux-packaging:', 1)[1]
        for text in (*builders.values(), review, ci):
            self.assertNotIn('secrets.', text)
            self.assertNotRegex(text, r'(?m)^    environment:')
            self.assertNotIn('deploy-pages', text)
            self.assertNotIn('gh release', text)
        for text in (*builders.values(), ci):
            self.assertIn('release/linux/install-nfpm.sh', text)
        for arch, text in builders.items():
            self.assertIn('release/linux/package-verified.sh', text)
            self.assertIn('steps.build.outputs.sha256', text)
            self.assertIn(f'linux-packages-{arch}', text)
            self.assertNotIn('linux-packages-review', text)
        self.assertIn('"$SHA256" dist/linux-packages arm64', builders['arm64'])
        self.assertIn('release/linux/merge-review-packages.sh', review)
        self.assertIn('linux-packages-review', review)
        self.assertIn('contents: read', ci)
        self.assertIn('release/linux/tests/rehearse-apt.sh', ci)


if __name__ == '__main__':
    unittest.main()
