# Non-publishing package-definition fixture; runtime/migration acceptance pending.
class CercanoAgent < Formula
  desc "Cercano agent (incomplete runtime fixture)"
  homepage "https://github.com/cercano-ai/Cercano"
  url "https://github.com/cercano-ai/Cercano/releases/download/v0.27.0/cercano-0.27.0-darwin-arm64.tar.gz"
  version "0.27.0"
  sha256 "18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0"
  license "MIT"

  depends_on macos: :monterey
  depends_on arch: :arm64

  def install
    bin.install "bin/cercano"
  end

  def caveats
    <<~EOS
      Package-definition fixture only; not release-ready.
      No bundled llama/model runtime; full runtime acceptance remains blocked.
      Installation does not start, stop, or restart an agent.
      Old-monolith migration requires isolated native acceptance before publication.
    EOS
  end

  test do
    assert_match "cercano v#{version}", shell_output("#{bin}/cercano --version")
  end
end
