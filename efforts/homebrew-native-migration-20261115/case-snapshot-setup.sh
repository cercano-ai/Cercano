#!/bin/bash
# Run exclusively through snapshot-recovery.py in NEW clone.
set -euo pipefail
R=/opt/cercano-snapshot-recovery-20261115
GOOD=/opt/cercano-migration-unpriv-20261115
OLD=/opt/cercano-migration-20261115
test ! -e "$R"
test -z "$(ls "$OLD/brew/Cellar")"
mkdir -m 755 "$R"
cp -R "$OLD/brew" "$R/brew"
mkdir -p "$R"/{home/.config/cercano,home/.cercano,tmp,cache,logs,artifacts}
cp "$GOOD"/artifacts/*.tar.gz "$R/artifacts/"
rm -rf "$R/brew/Library/Taps/native/homebrew-migration" "$R/brew/Library/Taps/migration/homebrew-cercano"
mkdir -p "$R/brew/Library/Taps/migration/homebrew-cercano/Formula"
sed "s|$GOOD|$R|g" "$GOOD/tap/Formula/cercano.rb" > "$R/brew/Library/Taps/migration/homebrew-cercano/Formula/cercano.rb"
printf 'port: "49384"\n' > "$R/home/.config/cercano/config.yaml"
printf 'synthetic-conversation\n' > "$R/home/.cercano/conversation-marker"
printf 'INERT-NOT-A-CREDENTIAL\n' > "$R/home/.cercano/credential-marker"
cp "$GOOD/brew/etc/homebrew/brew.env" "$R/brew/etc/homebrew/brew.env"
cp "$GOOD/safety.sb" "$R/safety.sb"
printf '\n(deny file-read-data file-write* (subpath "%s"))\n' "$GOOD" >> "$R/safety.sb"
sed "s|$GOOD|$R|g" "$GOOD/brew-fixture.sh" > "$R/brew-fixture.sh"
chown -R 5501:5501 "$R"; chmod -R go-w "$R"
chown root:wheel "$R" "$R/safety.sb" "$R/brew-fixture.sh"
chmod 755 "$R"; chmod 444 "$R/safety.sb"; chmod 555 "$R/brew-fixture.sh"
cmp "$GOOD/brew/Library/Homebrew/sandbox.rb" "$R/brew/Library/Homebrew/sandbox.rb"
cmp "$GOOD/brew/Library/Homebrew/extend/os/mac/sandbox.rb" "$R/brew/Library/Homebrew/extend/os/mac/sandbox.rb"
id _cercanorehearsal
"$R/brew-fixture.sh" trust migration/cercano
"$R/brew-fixture.sh" install --formula migration/cercano/cercano
shasum -a 256 "$R"/artifacts/*.tar.gz "$R/safety.sb" "$R/brew/Cellar/cercano/0.25.2/bin/"* "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"*
ls -l "$R/brew/bin/"cercano*
sync
