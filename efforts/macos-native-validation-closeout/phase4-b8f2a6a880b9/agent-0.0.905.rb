# Non-publishing package-definition fixture; runtime/migration acceptance pending.
class CercanoAgent < Formula
  desc "Cercano agent (incomplete runtime fixture)"
  homepage "https://github.com/cercano-ai/Cercano"
  url "file:///private/tmp/cnp.h8sjTc/phase4-b8f2a6a880b9/native-evidence/cercano-0.0.905.tar.gz"
  version "0.0.905"
  sha256 "8dc216182a03fb69d4c335912b21be70739024194cb6d6066a20f7a506244dfa"
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
