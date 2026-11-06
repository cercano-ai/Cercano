"""Shared trusted-set contract. No environment/CLI override for acceptance."""
from pathlib import Path
import re
import render_formula as r

# Change only after reviewed native/runtime acceptance evidence is recorded.
PUBLICATION_ACCEPTED = False
NAMES = ('cercano-agent.rb', 'cercano-cli.rb', 'cercano.rb')


def require_acceptance():
    if not PUBLICATION_ACCEPTED:
        raise r.ValidationError('Homebrew publication is disabled: native/runtime acceptance pending')


def expected(version, digest):
    r.parse_version(version)
    r.parse_digest(digest)
    return {name: r.render(Path(__file__).with_name(name + '.in').read_text(),
                          version, f'cercano-{version}-darwin-arm64.tar.gz', digest)
            for name in NAMES}


def reviewed(directory, version, digest):
    desired = expected(version, digest)
    for name, text in desired.items():
        path = Path(directory) / name
        if path.is_symlink() or path.read_text() != text:
            raise r.ValidationError('Reviewed formula set differs from trusted templates: ' + name)
    return desired


def fields(text):
    result = {}
    for key in ('version', 'sha256', 'url'):
        values = re.findall(r'^\s*' + key + r'\s+"([^"\n]+)"\s*$', text, re.M)
        if len(values) != 1:
            raise r.ValidationError('Invalid existing formula field: ' + key)
        result[key] = values[0]
    r.parse_version(result['version'])
    r.parse_digest(result['sha256'])
    return result


def decision(existing, desired, version, digest):
    """Only absent, complete trusted split, or older legacy monolith is eligible."""
    if desired != expected(version, digest):
        raise r.ValidationError('Incomplete or tampered desired formula set')
    if not existing:
        return 'update'
    if set(existing) not in ({'cercano.rb'}, set(NAMES)):
        raise r.ValidationError('Incomplete existing formula set')
    metadata = [fields(text) for text in existing.values()]
    if any(item != metadata[0] for item in metadata):
        raise r.ValidationError('Mixed existing formula set')
    old = metadata[0]
    if set(existing) == {'cercano.rb'}:
        # Read-only identification of the historical two-command keg. Never run
        # its Ruby/hooks. A lone new convenience formula is an incomplete set.
        if '    bin.install "bin/cercano", "bin/cercano-cli"' not in existing['cercano.rb']:
            raise r.ValidationError('Incomplete split, not a historical monolith')
    if set(existing) == set(NAMES) and existing != expected(old['version'], old['sha256']):
        raise r.ValidationError('Tampered existing formula set')
    a, b = (tuple(map(int, v.split('.'))) for v in (old['version'], version))
    if a > b:
        raise r.ValidationError('Refusing downgrade')
    if a == b:
        if existing != desired:
            raise r.ValidationError('Same version mutation refused')
        return 'unchanged'
    return 'update'
