"""Offline regression checks; no host, credentials, or history access."""
import ast
import pathlib
import unittest

SOURCE = pathlib.Path(__file__).with_name('everyday-launcher-compiled.py')

class ClosureTests(unittest.TestCase):
    def test_intentional_quit_closes_descriptor(self):
        tree = ast.parse(SOURCE.read_text())
        fn = next(n for n in ast.walk(tree) if isinstance(n, ast.FunctionDef) and n.name == 'intentional_quit')
        class Process:
            def poll(self): return 0
        class Terminal:
            p = Process()
            data = b''
            closed = False
            def send(self, s): pass
            def pump(self): pass
            def stop(self): self.closed = True
        t = Terminal()
        import time
        namespace = {'time': time, 'terminals': [t]}
        exec(compile(ast.Module(body=[fn], type_ignores=[]), str(SOURCE), 'exec'), namespace)
        namespace['intentional_quit'](t)
        self.assertTrue(t.closed, 'quit terminal removed from cleanup without closing PTY')

    def test_probe_does_not_expect_obsolete_retention(self):
        self.assertNotIn("blocked: ordinary quit retains ownership", SOURCE.read_text())

    def test_help_documents_detach_exception(self):
        root = SOURCE.resolve().parents[2]
        text = (root / 'source/clients/cli/internal/ui/dev_host.go').read_text()
        self.assertIn('exit after explicit detach retains ownership', text)

if __name__ == '__main__': unittest.main()
