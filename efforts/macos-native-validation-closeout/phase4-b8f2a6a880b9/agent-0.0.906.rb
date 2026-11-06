# Non-publishing package-definition fixture; runtime/migration acceptance pending.
class CercanoAgent < Formula
  desc "Cercano agent (incomplete runtime fixture)"
  homepage "https://github.com/cercano-ai/Cercano"
  url "file:///private/tmp/cnp.h8sjTc/phase4-b8f2a6a880b9/native-evidence/cercano-0.0.906.tar.gz"
  version "0.0.906"
  sha256 "76f9790daa7b14608cbbeab1bf305c6afe8b12703e749144badbc2435c0ed1db"
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
