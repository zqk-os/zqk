class Zqk < Formula
  desc "Kernel and orchestration CLI for AI-human hybrid software engineering"
  homepage "https://github.com/zqk-os/zqk"
  version "0.2.0"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_arm64.tar.gz"
      sha256 "eb88d5a03ef2ba5287ffa764741807cf4c401ab32c5568b65d85a0650b5362d1"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_darwin_amd64.tar.gz"
      sha256 "88320572e534880eebe6983eeebec7b381225f5d3b8d9e243c0b1ced263f9a67"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_arm64.tar.gz"
      sha256 "cbb606d07371ed80ca2ef2c9cea3dc98750b6bac1822f6d2b7f649044b5fe5e6"
    else
      url "https://github.com/zqk-os/zqk/releases/download/v#{version}/zqk_#{version}_linux_amd64.tar.gz"
      sha256 "f88c5b34f22566b473dc0b578f75d6334c877e3087761c39ef5540df8d8bc4a3"
    end
  end

  def install
    bin.install "zqk"
  end

  test do
    system "#{bin}/zqk", "--help"
  end
end
