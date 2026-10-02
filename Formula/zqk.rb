class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "0.1.0-beta.14"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_arm64.tar.gz"
      sha256 "25761e5947189dae1b985842e9067b2b2c8720ef621009e77ce61fd60d402005"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_darwin_amd64.tar.gz"
      sha256 "97b7782eb92db86405d45ad794370cd8e54726e1d7ddd0e7a0e0360008c4721b"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_arm64.tar.gz"
      sha256 "5db78f101ae5b2b67c55586065ccde0eda2fbf273d63c8a36e457c4d203ca1fe"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk-community_#{version}_linux_amd64.tar.gz"
      sha256 "019dd0a7ef37e8bb99a6a857c6b5a130dc3677d708cae529c84fcde39590be85"
    end
  end

  def install
    bin.install "zqk-community" => "zqk"
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
