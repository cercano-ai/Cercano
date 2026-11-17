#!/bin/sh
# Lifecycle test for one package family, run as root inside a throwaway
# container. /pkgs holds the built packages. Usage: container-lifecycle.sh <deb|rpm|apk|arch>
# Installs, checks both commands, starts the agent as a normal user, removes
# the convenience package (components stay), then removes everything.
set -u
fam=$1
echo "== $(. /etc/os-release; echo "$PRETTY_NAME") [$fam]"
case $fam in
  deb)  export DEBIAN_FRONTEND=noninteractive
        apt-get update -qq >/dev/null
        apt-get install -y -qq /pkgs/cercano*.deb 2>&1 | grep -E "^E:"
        q() { dpkg-query -W -f='${Package} ${Version}\n' cercano cercano-agent cercano-cli 2>&1; }
        rm_meta() { apt-get remove -y -qq cercano >/dev/null 2>&1; }
        rm_all() { apt-get purge -y -qq cercano-agent cercano-cli 2>&1 | grep -E "^E:"; } ;;
  rpm)  if command -v dnf >/dev/null; then I="dnf install -y -q"; R="dnf remove -y -q"
        else I="zypper -n -q --no-gpg-checks install --allow-unsigned-rpm"; R="zypper -n -q rm"; fi
        $I /pkgs/cercano*.rpm 2>&1 | grep -iE "error|problem" | head -5
        q() { rpm -q cercano cercano-agent cercano-cli; }
        rm_meta() { rpm -e cercano; }
        rm_all() { $R cercano-agent cercano-cli >/dev/null 2>&1; } ;;
  apk)  apk add -q --allow-untrusted /pkgs/cercano*.apk 2>&1 | head -5
        q() { apk info -v 2>/dev/null | grep '^cercano'; }
        rm_meta() { apk del -q cercano; }
        rm_all() { apk del -q cercano-agent cercano-cli; } ;;
  arch) pacman -U --noconfirm /pkgs/cercano*.pkg.tar.zst 2>&1 | grep -iE "^error"
        q() { pacman -Q cercano cercano-agent cercano-cli 2>/dev/null; }
        rm_meta() { pacman -R --noconfirm cercano >/dev/null; }
        rm_all() { pacman -R --noconfirm cercano-agent cercano-cli >/dev/null; } ;;
  *)    echo "unknown family: $fam"; exit 2 ;;
esac
q
echo "-- commands"
cercano --version; cercano-cli --version
ls -l /usr/bin/cercano /usr/bin/cercano-cli | awk '{print $1, $3, $4, $NF}'
echo "-- agent as a normal user"
id tester >/dev/null 2>&1 || useradd -m tester 2>/dev/null || adduser -D tester
su tester -c 'cd ~ && { cercano agent > agent.log 2>&1 & pid=$!; sleep 6; grep -E "Server listening|ERROR|panic" agent.log; kill $pid; }'
echo "-- remove the convenience package: components stay"
rm_meta; q
echo "-- remove the components: nothing left"
rm_all
ls -d /usr/bin/cercano* /usr/share/doc/cercano* /usr/share/licenses/cercano* 2>/dev/null || echo "no cercano files left"
