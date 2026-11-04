# Offline DSL fixture: no Homebrew or process invocation.
require "minitest/autorun"
class Formula
  class << self
    attr_accessor :requirements
    def desc(*) end
    def homepage(*) end
    def url(*) end
    def version(*) end
    def sha256(*) end
    def license(*) end
    def depends_on(value)
      self.requirements ||= []
      self.requirements << value
    end
    def test(&block) end
  end
end
%w[cercano cercano-agent cercano-cli].each { |name| load File.join(__dir__, "#{name}.rb.in") }
class FormulaOwnershipTest < Minitest::Test
  def test_exclusive_command_ownership
    {Cercano => [], CercanoAgent => ["bin/cercano"], CercanoCli => ["bin/cercano-cli"]}.each do |klass, expected|
      installed = []
      bin = Object.new
      bin.define_singleton_method(:install) { |*files| installed.concat(files) }
      formula = klass.new
      formula.define_singleton_method(:bin) { bin }
      formula.install
      assert_equal expected, installed
      refute formula.respond_to?(:post_install)
      refute formula.respond_to?(:service)
    end
  end
  def test_dependencies
    assert_includes Cercano.requirements, "cercano-agent"
    assert_includes Cercano.requirements, "cercano-cli"
    [CercanoAgent, CercanoCli].each do |klass|
      assert_equal [{macos: :monterey}, {arch: :arm64}], klass.requirements
    end
  end
  def test_no_state_or_process_hooks
    %w[cercano cercano-agent cercano-cli].each do |name|
      source = File.read(File.join(__dir__, "#{name}.rb.in"))
      refute_match(/post_install|restart-after-upgrade|launchd|system\s|kill\s|service do|File\.(write|delete)|rm_rf/, source)
    end
  end
end
