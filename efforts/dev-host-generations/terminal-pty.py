#!/usr/bin/env python3
"""Invoked by TestDevHostTerminalPTY; private tagged synthetic fixtures ONLY."""
import errno, fcntl, json, os, pty, re, select, signal, struct, subprocess, sys, termios, time
from pathlib import Path
binary, root = sys.argv[1], Path(sys.argv[2])

def control(path, action, *args, ok=True):
    p = subprocess.run([binary, '--dev-host', str(path), '--dev-host-action', action, *args], capture_output=True, text=True, timeout=15)
    if ok and p.returncode: raise AssertionError(p.stdout+p.stderr)
    if not ok and not p.returncode: raise AssertionError('expected refusal: '+p.stdout)
    return p

class Terminal:
    def __init__(self, selection):
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 48, 160, 0, 0))
        self.fd, self.output = master, b''
        self.p = subprocess.Popen([binary, '--dev-host', str(selection)], stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
    def pump(self):
        if select.select([self.fd], [], [], .1)[0]:
            try: data = os.read(self.fd, 65536)
            except OSError as e:
                if e.errno == errno.EIO: data = b''
                else: raise
            self.output += data
            # Answer terminal capability queries, not application prompts.
            if b'\x1b[c' in data: os.write(self.fd,b'\x1b[?1;2c')
            if b'\x1b[6n' in data: os.write(self.fd,b'\x1b[1;1R')
    def wait(self, text, timeout=15):
        end = time.monotonic()+timeout
        while time.monotonic()<end:
            self.pump()
            plain = re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]',b'',self.output).decode(errors='replace')
            if text in plain: return
        raise AssertionError('missing '+text+'\n'+self.output.decode(errors='replace')[-12000:])
    def send(self,text):
        self.output=b''
        os.write(self.fd,text.encode()+b'\r')
    def stop(self):
        if self.p.poll() is None: self.p.kill()
        self.p.wait(timeout=5)
        os.close(self.fd)

terminals=[]
try:
    a,b=root/'a.json',root/'b.json'
    t=Terminal(a); terminals.append(t)
    t.wait('durable fixture history')
    t.send('/version'); t.wait('fixture-TUI-unchanged'); t.wait('fixture-build-exact')
    os.write(t.fd,b'\x1b'); time.sleep(.3)
    t.send('launch independent')
    deadline=time.monotonic()+15
    while not (root/'entered').exists() and time.monotonic()<deadline: t.pump()
    assert (root/'entered').exists(), 'independent work never entered synthetic barrier'
    assert not (root/'release').exists(), 'barrier already released'
    assert t.p.poll() is None, 'terminal ended before active-work disconnect'
    t.stop(); terminals.remove(t)
    # Actual disconnect DURING independent work, not a cancellation after EOF.
    time.sleep(.3)
    refused=control(a,'release',ok=False)
    assert 'busy' in refused.stderr.lower() or 'work' in refused.stderr.lower(), refused.stderr
    print('PASS: PTY disconnected while independent work barrier held; release refused',flush=True)
    (root/'release').write_text('explicit completion')
    t=Terminal(a); terminals.append(t); t.wait('fixture-build-exact')
    t.send('/host detach'); t.wait('session retained')
    time.sleep(.3)
    t.send('/host transfer '+str(b)+' pty-handover'); t.wait('Completed; attachment requires explicit')
    t.send('/host reconnect'); time.sleep(.5)
    t.wait('fixture-build-exact')
    t.send('/version'); t.wait('fixture-TUI-unchanged'); t.wait('fixture-build-B'); t.wait('fixture-host-B')
    os.write(t.fd,b'\x1b'); time.sleep(.3)
    control(a,'retire')
    stale=subprocess.run([binary,'--dev-host',str(a)],capture_output=True,text=True,timeout=10)
    assert stale.returncode != 0, 'stale A reconnect succeeded'
    t.send('B remains usable'); t.wait('fixture-build-B')
    print('PASS: same TUI moved A to B; exact B identity shown; A retired; stale A refused; B usable',flush=True)
finally:
    for t in terminals: t.stop()
