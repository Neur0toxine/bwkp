cask "bwkp" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.3" # x-release-please-version
  sha256 arm:          "2edf76442b1ab0a7575cb0ab2cc8eea1bea566c5bc9e191afc8c9492b05f044c", # bwkp-release-arm64
         arm64_linux:  "2edf76442b1ab0a7575cb0ab2cc8eea1bea566c5bc9e191afc8c9492b05f044c",
         x86_64:       "03c2f5e8bfb446ce2487dbe5ffa1bc4fcc2fb067fa4a168d5b62ebd46456e3f5", # bwkp-release-amd64
         x86_64_linux: "03c2f5e8bfb446ce2487dbe5ffa1bc4fcc2fb067fa4a168d5b62ebd46456e3f5"

  url "https://github.com/Neur0toxine/bwkp/releases/download/v#{version}/bwkp_v#{version}_macos-#{arch}.tar.gz"
  name "bwkp"
  desc "Transfer records between Bitwarden or Vaultwarden and KeePassXC"
  homepage "https://github.com/Neur0toxine/bwkp"

  depends_on macos: ">= :ventura"

  binary "bwkp"
end
