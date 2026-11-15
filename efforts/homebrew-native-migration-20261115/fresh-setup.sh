#!/bin/bash
set -euo pipefail
R=/opt/cercano-migration-unpriv-20261115
O=/opt/cercano-migration-20261115
U=_cercanorehearsal
# Fixture creation only; refuse collisions instead of altering existing identities.
test ! -e "$R"
! dscl . -read /Users/$U >/dev/null 2>&1
! dscl . -read /Groups/$U >/dev/null 2>&1
! dscl . -list /Users UniqueID | awk '{print $NF}' | grep -qx 5501
! dscl . -list /Groups PrimaryGroupID | awk '{print $NF}' | grep -qx 5501
dscl . -create /Groups/$U
dscl . -create /Groups/$U PrimaryGroupID 5501
dscl . -create /Users/$U
dscl . -create /Users/$U UniqueID 5501
dscl . -create /Users/$U PrimaryGroupID 5501
dscl . -create /Users/$U UserShell /usr/bin/false
dscl . -create /Users/$U NFSHomeDirectory "$R/home"
dscl . -create /Users/$U IsHidden 1
# No password, authentication authority, keychain or login session is provisioned.
mkdir -m 755 "$R"
mkdir -p "$R"/{home/.config/cercano,home/.cercano,tmp,cache,logs,artifacts,inspect,tap/Formula}
cp -R "$O/brew" "$R/brew"
cp "$O/artifacts/"*.tar.gz "$R/artifacts/"
cp "$O/safety.sb" "$R/safety.sb"
# Strengthen only: retain every original rule, additionally protect old fixture.
printf '\n(deny file-read-data file-write* (subpath "%s"))\n' "$O" >> "$R/safety.sb"
# Remove only copied fixture-specific taps; no existing source is modified.
rm -rf "$R/brew/Library/Taps/native/homebrew-migration" "$R/brew/Library/Taps/migration/homebrew-cercano"
mkdir -p "$R/brew/Library/Taps/migration/homebrew-cercano/Formula"
sed "s|$O/artifacts|$R/artifacts|" "$O/tap/Formula/cercano.rb" > "$R/tap/Formula/cercano.rb"
cp "$R/tap/Formula/cercano.rb" "$R/brew/Library/Taps/migration/homebrew-cercano/Formula/cercano.rb"
printf 'port: "49382"\n' > "$R/home/.config/cercano/config.yaml"
printf 'synthetic-conversation\n' > "$R/home/.cercano/conversation-marker"
printf 'INERT-NOT-A-CREDENTIAL\n' > "$R/home/.cercano/credential-marker"
printf '%s\n' HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 > "$R/brew/etc/homebrew/brew.env"
chown -R 5501:5501 "$R"
chmod -R go-w "$R"
# Profile is immutable to fixture identity (no write permission on its parent).
chown root:wheel "$R" "$R/safety.sb"
chmod 755 "$R"; chmod 444 "$R/safety.sb"
id "$U"
dscl . -read /Users/$U
for g in admin staff root wheel; do dsmemberutil checkmembership -U "$U" -G "$g"; done
stat -f '%u:%g %Sp %N' "$R" "$R/home" "$R/brew" "$R/tmp" "$R/artifacts"
shasum -a 256 "$R/artifacts/"*.tar.gz "$R/safety.sb" "$O/safety.sb" "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"*
# Verify copied Homebrew guard sources are byte-identical.
cmp "$O/brew/Library/Homebrew/sandbox.rb" "$R/brew/Library/Homebrew/sandbox.rb"
cmp "$O/brew/Library/Homebrew/extend/os/mac/sandbox.rb" "$R/brew/Library/Homebrew/extend/os/mac/sandbox.rb"
for v in 0.25.2 0.27.0; do
 mkdir "$R/inspect/$v"
 tar -xzf "$R/artifacts/cercano-$v-darwin-arm64.tar.gz" -C "$R/inspect/$v"
 find "$R/inspect/$v" -type f -perm +111 -exec codesign --verify --strict --verbose=2 {} \;
 find "$R/inspect/$v" -type f -perm +111 -exec codesign -dv --verbose=2 {} \;
done
chown -R 5501:5501 "$R/inspect"
