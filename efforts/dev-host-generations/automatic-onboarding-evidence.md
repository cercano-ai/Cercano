# Automatic development onboarding (branch-only)

Continues checkpoint `4ff00a5694618b0dcfd7cfdf712d1f43e7bd3d34` on
`fix/dev-config-onboarding`.

Reproduction used the checkpoint's shell and Python launcher copied with
`git show` into a temporary fake repository, synthetic HOME/config and absent
default state. It exited 78 and left state absent despite current config.
No user config, installed launcher, production startup or credentials were used.

The source launcher now serializes setup with a private persistent sibling lock,
publishes a temporary build-only bootstrap outside the absent state, invokes
its explicit early `prepare-development`, and then resumes normal launch.
Preparation continues to use the existing Go implementation for source discovery,
exclusive creation and config copying. Existing state passes through the existing
read-only compatibility checks; no repair/adoption is attempted. Bootstrap output
is not relayed. Temporary bootstrap publication is cleaned, but failed partial
state is preserved for inspection.

Verification in this worktree:

- `bash source/server/scripts/cercano-launcher_test.sh`: 15 tests passed.
  Uses actual source shell/Python orchestration with synthetic publisher/server/
  CLI fixtures, not production launch. Covers automatic absent default and custom
  state, four concurrent first launches (exactly one preparation), private modes,
  missing source, build failure, partial/legacy/incompatible refusal, unchanged
  originals/legacy sentinel, no secret output, cleanup, and subsequent reuse of
  independently edited config. Existing selector/runtime safety tests also pass.
- `go test ./cmd/cercano -run PrepareDevelopment -count=1` from `source/server`:
  passed, exercising the actual early Go preparation implementation independently.

No push/landing or installed-launcher modification was performed. Ordinary
installed `cercano` is not claimed fixed by this branch. Once this source template
is integrated through a separately approved update, default first launch needs no
provisioning command or state variable. A readable current config and build tools
are still prerequisites. Missing config must be configured, and partial or
incompatible destinations require deliberate diagnosis/new-state selection;
local modelhost and runtime credential prerequisites remain as documented.
