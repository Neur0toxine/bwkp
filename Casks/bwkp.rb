cask "bwkp" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.2" # x-release-please-version
  sha256 arm:          "269d77735b5a4496b842fafb2a5a74ab57f85198d9bb23fae8f415ab825226e4", # bwkp-release-arm64
         arm64_linux:  "269d77735b5a4496b842fafb2a5a74ab57f85198d9bb23fae8f415ab825226e4",
         x86_64:       "457f845c7b620d787427356c535845abcf6cbdd83524619b8ed4c747827e617a", # bwkp-release-amd64
         x86_64_linux: "457f845c7b620d787427356c535845abcf6cbdd83524619b8ed4c747827e617a"

  url "https://github.com/Neur0toxine/bwkp/releases/download/v#{version}/bwkp_v#{version}_macos-#{arch}.tar.gz"
  name "bwkp"
  desc "Transfer records between Bitwarden or Vaultwarden and KeePassXC"
  homepage "https://github.com/Neur0toxine/bwkp"

  depends_on macos: ">= :ventura"

  binary "bwkp"
end
