#!/usr/bin/env python3
"""Scoped continuation transport; all guest commands recorded by rehearse.py."""
import pathlib,sys,base64
from rehearse import guest
R='/opt/cercano-migration-unpriv-20261115'
def deploy(name,content,mode='444'):
    b=base64.b64encode(content.encode()).decode()
    guest(f"set -eu; echo '{b}' | base64 -D > '{R}/{name}'; chown root:wheel '{R}/{name}'; chmod {mode} '{R}/{name}'")
if sys.argv[1]=='deploy':
    deploy('probes-v2.rb',pathlib.Path(__file__).with_name('fresh-probes.rb').read_text())
    deploy('brew-fixture.sh',pathlib.Path(__file__).with_name('unpriv-brew-fixture.sh').read_text(),'555')
else:
    guest(sys.stdin.read())
