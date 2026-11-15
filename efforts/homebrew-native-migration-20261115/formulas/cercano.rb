# Non-publishing package-definition fixture; runtime/migration acceptance pending.
class Cercano < Formula
  desc "Cercano CLI and agent convenience dependencies"
  homepage "https://github.com/cercano-ai/Cercano"
  url "https://github.com/cercano-ai/Cercano/releases/download/v0.27.0/cercano-0.27.0-darwin-arm64.tar.gz"
  version "0.27.0"
  sha256 "18c68ccc69662da6d18b8017a3eba7eaebc67e057065b90bb66deba2838d2aa0"
  license "MIT"

  depends_on macos: :monterey
  depends_on arch: :arm64

  # Coordinated release set, not an atomic upgrade or an exact-version lock.
  depends_on "cercano-agent"
  depends_on "cercano-cli"

  def install
    # Keep the keg nonempty without owning commands or runtime resources.
    pkgshare.install "README.txt"
  end

  def caveats
    <<~EOS
      Package-definition fixture only; not release-ready.
      Agent owns cercano; client owns cercano-cli. No bundled llama runtime.
      Old-monolith migration and partial upgrades require native acceptance.
      Installation does not start, stop, or restart an agent.
    EOS
  end

  test do
    assert_predicate Formula["cercano-agent"].opt_bin/"cercano", :executable?
    assert_predicate Formula["cercano-cli"].opt_bin/"cercano-cli", :executable?
  end
end
