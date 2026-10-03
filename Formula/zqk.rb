class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "0.1.0-beta.19"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_arm64.tar.gz"
      sha256 "4c1ce03d022a515f5c09a1187a3b8e490192fe9bc3111171bae6ffb45dcc5d3d"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_amd64.tar.gz"
      sha256 "750bfd27704c09a8d4ecf9d9455c7addb2c4db14f07eeb0768da1314a20fd0de"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_arm64.tar.gz"
      sha256 "5edd86d67735492dc9cc923bb0889ec0c06f19ed9647bf337a7eedd224b11cea"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_amd64.tar.gz"
      sha256 "2292fae706c051bb14bb2512845c3d4a502cefcb365dec4f38a4d4430e2c0fe7"
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
