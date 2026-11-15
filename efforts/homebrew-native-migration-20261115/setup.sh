#!/bin/bash
set -euo pipefail
R=/opt/cercano-migration-20261115
mkdir -p "$R"/{home/.config/cercano,home/.cercano,tmp,cache,logs,brew/etc/homebrew,tap/Formula}
chmod 755 "$R" "$R/home"
printf 'port: "49381"\n' > "$R/home/.config/cercano/config.yaml"
printf 'synthetic-conversation\n' > "$R/home/.cercano/conversation-marker"
printf 'INERT-NOT-A-CREDENTIAL\n' > "$R/home/.cercano/credential-marker"
printf '%s\n' HOMEBREW_NO_CLEANUP_FORMULAE=cercano,cercano-agent HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 > "$R/brew/etc/homebrew/brew.env"
# No historical hook may reach any network endpoint; the no-agent case needs no network.
cat > "$R/safety.sb" <<'EOF'
(version 1)
(allow default)
(deny network*)
(deny signal)
(deny file-read* file-write* (subpath "/Users/test") (subpath "/Library/Keychains") (subpath "/opt/homebrew"))
EOF
chown -R 501:20 "$R"
sudo -u test env -i HOME="$R/home" PATH=/usr/bin:/bin:/usr/sbin:/sbin HOMEBREW_CACHE="$R/cache" HOMEBREW_LOGS="$R/logs" HOMEBREW_TEMP="$R/tmp" "$R/brew/bin/brew" config
shasum -a 256 "$R/home/.config/cercano/config.yaml" "$R/home/.cercano/"*
