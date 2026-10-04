class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "0.1.0-beta.20"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_arm64.tar.gz"
      sha256 "0589ee71ef9a17e4a3adc3e10611386f87157273700a51034dd24e75cd2cf236"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_amd64.tar.gz"
      sha256 "648689d3e85e3e0e4eb359e8d5daa5f99dd3984e4d794ab069ab7c2bea5bae6f"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_arm64.tar.gz"
      sha256 "bb26f20a9862df6ea817d98ac6229c930e4dc0177f2012e78df140f2ae0b0082"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_amd64.tar.gz"
      sha256 "850912f19c500e27ba3ea03fc2a967dfd77db84bd31750e4774ac72fdb487bea"
    end
  end

  def install
    if File.exist?("zqk")
      bin.install "zqk"
    else
      bin.install "zqk-community" => "zqk"
    end
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
