#!/bin/sh
# Upgrade test for one package family, run as root inside a throwaway
# container. /old and /new hold two versions' packages; NEW is the newer
# version. Usage: container-upgrade.sh <deb|rpm|apk|arch> <new version>
# Installs the old set, checks that the convenience package alone is refused
# while the components are older, then upgrades all three.
set -u
fam=$1 new=$2
echo "== $(. /etc/os-release; echo "$PRETTY_NAME") [$fam]"
case $fam in
  deb)  export DEBIAN_FRONTEND=noninteractive; apt-get update -qq >/dev/null
        q() { dpkg-query -W -f='${Package} ${Version}  ' cercano cercano-agent cercano-cli; echo; }
        inst() { apt-get install -y -qq "$@" 2>&1 | grep -E "^E:|Depends:" | head -3; }
        old=/old/*.deb; meta=/new/cercano_${new}_all.deb; all=/new/*.deb ;;
  rpm)  if command -v dnf >/dev/null; then I="dnf install -y -q"; else I="zypper -n -q --no-gpg-checks install --allow-unsigned-rpm"; fi
        q() { rpm -q cercano cercano-agent cercano-cli | tr '\n' ' '; echo; }
        inst() { $I "$@" 2>&1 | grep -iE "error|problem|nothing provides" | head -3; }
        old=/old/*.rpm; meta=/new/cercano-${new}-1.noarch.rpm; all=/new/*.rpm ;;
  apk)  q() { apk info -v 2>/dev/null | grep '^cercano' | tr '\n' ' '; echo; }
        inst() { apk add -q --allow-untrusted "$@" 2>&1 | head -3; }
        old=/old/*.apk; meta=/new/cercano_${new}_noarch.apk; all=/new/*.apk ;;
  arch) q() { pacman -Q cercano cercano-agent cercano-cli 2>/dev/null | tr '\n' ' '; echo; }
        inst() { pacman -U --noconfirm "$@" 2>&1 | grep -iE "^error|cannot resolve" | head -3; }
        old=/old/*.pkg.tar.zst; meta=/new/cercano-${new}-1-any.pkg.tar.zst; all=/new/*.pkg.tar.zst ;;
  *)    echo "unknown family: $fam"; exit 2 ;;
esac
echo "-- install the old set"; inst $old; q
echo "-- convenience package alone to $new: expect a refusal"; inst $meta; q
echo "-- upgrade all three"; inst $all; q
cercano --version; cercano-cli --version
