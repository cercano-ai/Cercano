"""Offline evidence assertions; never starts a process or contacts a provider."""
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).parent
spec = importlib.util.spec_from_file_location('compiled', ROOT/'everyday-launcher-compiled.py')
harness = importlib.util.module_from_spec(spec)
spec.loader.exec_module(harness)


class WorkerBirthTests(unittest.TestCase):
    def test_retained_trace_reproduces_old_false_positive(self):
        events = [json.loads(x) for x in (ROOT/'worker-birth-regression/retained-worker.jsonl').read_text().splitlines()]
        boundary = json.loads((ROOT/'worker-birth-regression/retained-boundary.json').read_text())['B_selection_mtime_ns']
        pre = {e['pid'] for e in events if e['unix_nano'] < boundary}
        later = [e for e in events if e['pid'] == 98189 and e['unix_nano'] > boundary]
        self.assertTrue(later)  # Old post-B StartTurn check passes.
        self.assertIn(98189, pre)
        self.assertFalse(any(e['event'] == 'worker-startup' for e in events))
        with self.assertRaisesRegex(AssertionError, 'reused'):
            harness.assert_new_worker(later, pre, boundary, later[0]['conversation'])

    def check(self, events, pre=()):
        return harness.assert_new_worker(events, pre, 100, 'A')

    def test_first_turn_is_not_startup(self):
        with self.assertRaisesRegex(AssertionError, 'child-entry'):
            self.check([self.turn()])

    def turn(self):
        return dict(event='worker-turn', pid=2, unix_nano=120, conversation='A')

    def birth(self, timestamp=110):
        return dict(event='worker-startup', pid=2, unix_nano=timestamp)

    def test_birth_before_boundary_rejected(self):
        with self.assertRaisesRegex(AssertionError, 'startup must follow'):
            self.check([self.birth(90), self.turn()])

    def test_birth_after_turn_rejected(self):
        with self.assertRaisesRegex(AssertionError, 'precede first turn'):
            self.check([self.birth(130), self.turn()])

    def test_reused_pid_rejected_even_with_startup(self):
        with self.assertRaisesRegex(AssertionError, 'reused'):
            self.check([self.birth(), self.turn()], [2])

    def test_new_child_accepted(self):
        birth, turns = self.check([self.birth(), self.turn()], [1])
        self.assertLess(birth['unix_nano'], turns[0]['unix_nano'])


if __name__ == '__main__':
    unittest.main()
