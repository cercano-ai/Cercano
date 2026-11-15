# Release template, not a publishable formula until all placeholders are filled.
# Use a signed/notarized two-binary archive that includes restart-after-upgrade.
class Cercano < Formula
  desc "AI-powered development tool with local/cloud model routing"
  homepage "https://github.com/cercano-ai/Cercano"
  url "https://github.com/cercano-ai/Cercano/releases/download/v0.25.2/cercano-0.25.2-darwin-arm64.tar.gz"
  version "0.25.2"
  sha256 "e57b4ff87d23241588df64e6f106b087775bb6c8939611fc9c540e907efef664"
  license "MIT"

  # Monterey is macOS 12, the deployment target the binaries are built against.
  # Homebrew refuses installation on older systems and on Intel rather than
  # letting a user install binaries that cannot run.
  depends_on macos: :monterey
  depends_on arch: :arm64

  def install
    bin.install "bin/cercano", "bin/cercano-cli"
  end

  # Homebrew invokes this after installing/linking the new keg. Never restart
  # from install itself: a download/unpack/install failure must leave the old
  # agent alone. The coordinator is a no-op when no owned agent is running.
  def post_install
    system bin/"cercano", "restart-after-upgrade"
  end

  def caveats
    <<~EOS
      Start the terminal client with cercano-cli.
      A successful upgrade restarts an existing agent after draining work.
      An absent agent is not started by installation or upgrade.
      If post-install restart fails, inspect the reported error and agent logs.
      Retry with: #{bin}/cercano restart-after-upgrade
      Manual restart from an attached client: /restart-agent
    EOS
  end

  # Runs with no credentials, no model downloads and no network, so every
  # command here must work offline. Each binary prints its own name, verified
  # against real builds.
  #
  # Deliberately does NOT invoke `restart-after-upgrade` without --help: that
  # command acts on a running agent, and `brew test` must not restart the
  # user's agent as a side effect.
  test do
    assert_match "cercano v#{version}", shell_output("#{bin}/cercano --version")
    assert_match "cercano-cli v#{version}", shell_output("#{bin}/cercano-cli --version")
    assert_match "Usage:", shell_output("#{bin}/cercano restart-after-upgrade --help")
  end
end
