#!/usr/bin/env python3
"""Invoked by TestDevHostTerminalPTY; private tagged synthetic fixtures ONLY."""
import errno, fcntl, json, os, pty, re, select, signal, struct, subprocess, sys, termios, time
from pathlib import Path
binary, root = sys.argv[1], Path(sys.argv[2])

def control(path, action, *args, ok=True):
    master, slave = pty.openpty()
    argv = [binary, '--dev-host', str(path), '--dev-host-action', action, *args]
    p = subprocess.Popen(argv, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    output = b''
    deadline = time.monotonic()+15
    try:
        while True:
            assert time.monotonic()<deadline, 'management CLI timeout'
            if not select.select([master], [], [], .1)[0]: continue
            try: data = os.read(master, 65536)
            except OSError as e:
                if e.errno != errno.EIO: raise
                break
            if not data: break
            output += data
        code = p.wait(timeout=5)
    finally:
        if p.poll() is None: p.kill(); p.wait()
        os.close(master)
    text = output.decode(errors='replace')
    with (root/'controls.log').open('a') as log:
        log.write(repr(argv)+'\n'+text+'\n')
    if ok and code: raise AssertionError(text)
    if not ok and not code: raise AssertionError('expected refusal: '+text)
    return subprocess.CompletedProcess(argv, code, text, text)

class Terminal:
    def __init__(self, selection):
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 48, 160, 0, 0))
        self.fd, self.output = master, b''
        self.transcript = open(root / ('pty-%d.log' % len(list(root.glob('pty-*.log')))), 'wb')
        self.p = subprocess.Popen([binary, '--dev-host', str(selection)], stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
    def pump(self):
        if select.select([self.fd], [], [], .1)[0]:
            try: data = os.read(self.fd, 65536)
            except OSError as e:
                if e.errno == errno.EIO: data = b''
                else: raise
            self.output += data
            self.transcript.write(data); self.transcript.flush()
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
        self.transcript.close()

def marker(name, terminal=None):
    deadline=time.monotonic()+60
    while not (root/name).exists():
        assert time.monotonic()<deadline, 'missing barrier '+name
        if terminal: terminal.pump()
        else: time.sleep(.01)

terminals=[]
try:
    a,b,y=root/'a.json',root/'b.json',root/'y.json'
    t=Terminal(a); terminals.append(t)
    t.wait('durable fixture history')
    t.send('launch independent')
    marker('entered',t)
    assert not (root/'release').exists()
    assert t.p.poll() is None
    t.stop(); terminals.remove(t)
    (root/'disconnected').write_text('X terminal physically closed')
    marker('b-ready')
    t=Terminal(y); terminals.append(t); t.wait('durable fixture history')
    # X has no terminal; management CLI targets its explicit selection while
    # the unrelated Y terminal remains attached. Never substitute a test RPC.
    refused=control(a,'transfer','--dev-host-destination',str(b),
                    '--dev-host-handover','busy-X',ok=False)
    (root/'busy-transfer.log').write_text(refused.stdout+refused.stderr)
    assert 'retained sessions or work prevent transition' in refused.stderr, refused.stderr
    t.send('/host detach'); t.wait('session retained')
    t.send('/host transfer '+str(b)+' pty-Y-handover'); t.wait('Completed; attachment requires explicit')
    assert not (root/'release').exists()
    (root/'transferred').write_text('Y transferred while X barrier held')
    marker('checked-running',t)
    t.send('/host reconnect'); t.wait('durable fixture history')
    t.send('/version'); t.wait('fixture-TUI-unchanged'); t.wait('fixture-build-B'); t.wait('fixture-host-B')
    os.write(t.fd,b'\x1b'); t.pump()
    (root/'release').write_text('explicit completion')
    marker('checked-completed',t)
    x=Terminal(a); terminals.append(x); x.wait('durable fixture history')
    x.send('/host detach'); x.wait('session retained')
    x.send('/host release'); x.wait('Completed')
    control(a,'retire')
    stale=subprocess.run([binary,'--dev-host',str(a)],capture_output=True,text=True,timeout=10)
    assert stale.returncode != 0, 'stale A reconnect succeeded'
    t.send('B remains usable'); t.wait('fixture-build-B')
    print('PASS: active X disconnect; B started; exact busy X transfer refusal; Y transferred with X running; durable X completion/notification; A released/retired; B usable',flush=True)
finally:
    for t in terminals: t.stop()
