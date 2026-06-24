# Homebrew formula for hc.
#
# This lives in a *tap* repo named "homebrew-tap" under your GitHub account, at
# Formula/hc.rb. Once published, users install with:
#
#     brew install <matt17r>/tap/hc
#
# Replace matt17r below and fill in the sha256 after you tag a release
# (see RELEASING.md).
class Hc < Formula
  desc "CLI for the healthchecks.io Management API"
  homepage "https://github.com/matt17r/healthchecks-cli"
  url "https://github.com/matt17r/healthchecks-cli/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "REPLACE_WITH_TARBALL_SHA256"
  license "MIT"
  head "https://github.com/matt17r/healthchecks-cli.git", branch: "main"

  depends_on "go" => :build

  def install
    # std_go_args builds to #{bin}/hc; inject the version into main.version.
    system "go", "build", *std_go_args(ldflags: "-X main.version=#{version}")
  end

  test do
    assert_match "hc #{version}", shell_output("#{bin}/hc version")
  end
end
