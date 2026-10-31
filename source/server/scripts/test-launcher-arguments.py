#!/usr/bin/env python3
"""Argument regression: import only, no host/config access."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('launcher', Path(__file__).with_name('cercano-launcher.py'))
launcher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(launcher)


class Arguments(unittest.TestCase):
    def test_resume_short(self):
        self.assertEqual(launcher.parse_args(['-r']), (None, ['-r'], None))

    def test_cli_contract(self):
        for args in [['--resume'], ['-resume=true'], ['-r=false', '--resume'],
                     ['-s'], ['--setup=false'], ['--mdtest', 'file with spaces.md'],
                     ['--mdtest=false'], ['--version=false', '-r'],
                     ['--mdtest', '--', '-sample.md'], ['file.md', '-r']]:
            with self.subTest(args=args):
                self.assertEqual(launcher.parse_args(args), (None, args, None))

    def test_selectors_still_preflight_and_normalize(self):
        self.assertEqual(launcher.parse_args(['-r', '-dev-host=selection']),
                         ('selection', ['--dev-host', 'selection', '-r'], None))
        for args in [['-r', '--dev-host='], ['-r', '--dev-host-action=list'],
                     ['--dev-host=a', 'file', '--dev-host=b'],
                     ['--', '--dev-host=a', '--dev-host=b'],
                     ['--resume=garbage'], ['--unknown']]:
            with self.subTest(args=args), self.assertRaises(ValueError):
                launcher.parse_args(args)

    def test_information_never_launches(self):
        for arg in ['--help', '-h', '--version', '--version=true']:
            self.assertIsNotNone(launcher.parse_args([arg])[2])


if __name__ == '__main__':
    unittest.main()
