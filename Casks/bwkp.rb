cask "bwkp" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.1" # x-release-please-version
  sha256 arm:          "0000000000000000000000000000000000000000000000000000000000000000", # bwkp-release-arm64
         arm64_linux:  "0000000000000000000000000000000000000000000000000000000000000000",
         x86_64:       "0000000000000000000000000000000000000000000000000000000000000000", # bwkp-release-amd64
         x86_64_linux: "0000000000000000000000000000000000000000000000000000000000000000"

  url "https://github.com/Neur0toxine/bwkp/releases/download/v#{version}/bwkp_v#{version}_macos-#{arch}.tar.gz"
  name "bwkp"
  desc "Transfer records between Bitwarden or Vaultwarden and KeePassXC"
  homepage "https://github.com/Neur0toxine/bwkp"

  depends_on macos: ">= :ventura"

  binary "bwkp"
end
