# Installing Cercano on Windows

Windows x64 remains **experimental, unsigned, and not a supported release**.
Windows may show SmartScreen/unknown-publisher warnings. Package submission
is not code signing or a claim of full Windows support. No local llama/model
runtime is bundled. See the [artifact and testing guide](windows-artifact.md)
for limitations and the ZIP download, an alternative without package-manager
installation.

## Chocolatey availability

`cercano`, `cercano-agent` and `cercano-cli` **0.27.1** are approved on the
Chocolatey Community Repository (checked 2026-11-27). Package version 0.27.1
contains application **0.27.0**, so `cercano --version` and
`cercano-cli --version` report `v0.27.0`.

- [cercano](https://community.chocolatey.org/packages/cercano)
- [cercano-agent](https://community.chocolatey.org/packages/cercano-agent)
- [cercano-cli](https://community.chocolatey.org/packages/cercano-cli)

Tested on Windows 11 (2026-11-27): a fresh install after uninstalling, an
upgrade from locally built 0.27.0 packages, and uninstall. The installed
executables matched the v0.27.0 release ZIP. That machine had run Cercano
before. Approval and these tests are not a claim of full Windows support.

## Install

With Chocolatey already installed, run in an elevated PowerShell:

```powershell
choco install cercano -y
```

Then launch from a normal terminal:

```powershell
cercano-cli
```

The client starts the local agent when needed. The dependency-only `cercano`
package owns no binaries; its components supply the commands:

| Package | Command | Dependencies |
| --- | --- | --- |
| `cercano-agent` | `cercano` (agent) | none |
| `cercano-cli` | `cercano-cli` (terminal client) | none |
| `cercano` | none | both components, minimum version 0.27.0 |

For individual installs instead of the convenience package:

```powershell
choco install cercano-agent -y
choco install cercano-cli -y
```

Install only the component you need. A CLI-only installation can connect to a
remote agent with `cercano-cli --address HOST:PORT`; it does not pull in a local
agent. For local interactive use, both components are needed.

## Upgrade

Save work and close Cercano before upgrading. Quitting the terminal client can
leave the agent and workers running: check for those processes too. The package
guards refuse changes while their processes are running; they do not stop or
restart them for you. If an upgrade is refused, resolve the reported running
processes before retrying; do not force replacement.

```powershell
choco upgrade cercano -y
```

The convenience package uses **minimum dependency versions**, not exact locks
or an atomic release set. When it upgrades, Chocolatey resolves dependencies
from available source versions; that does not guarantee identical component
versions. If the convenience package is already current, do not assume the
command refreshed independently updated components. To explicitly upgrade both
(or only the component you installed):

```powershell
choco upgrade cercano-agent cercano-cli -y
```

Do not use `--ignore-dependencies` for the convenience package. Moderation or
repository lag can leave dependencies unavailable; wait rather than bypassing
those checks. After a successful upgrade, launch `cercano-cli` again and check:

```powershell
cercano --version
cercano-cli --version
```

Keep configuration and conversations intact during upgrades. If a prior ZIP,
source build, or other installation is also on PATH, use `Get-Command cercano,cercano-cli -All`
to inspect command resolution; upgrading Chocolatey packages does not replace
those other copies. No automatic migration from other installation methods is
prescribed here.
