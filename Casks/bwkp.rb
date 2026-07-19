cask "bwkp" do
  arch arm: "arm64", intel: "amd64"

  version "0.1.2" # x-release-please-version
  sha256 arm:          "e1e4df70fa8bc00f037df711e7f8e4c6a1d0feb9484cb9f884d23f530c67eea5", # bwkp-release-arm64
         arm64_linux:  "e1e4df70fa8bc00f037df711e7f8e4c6a1d0feb9484cb9f884d23f530c67eea5",
         x86_64:       "51a2e9a0fc3a41f184ed5b4f3fb8238b4353fed3960136722e89b15e3a87641b", # bwkp-release-amd64
         x86_64_linux: "51a2e9a0fc3a41f184ed5b4f3fb8238b4353fed3960136722e89b15e3a87641b"

  url "https://github.com/Neur0toxine/bwkp/releases/download/v#{version}/bwkp_v#{version}_macos-#{arch}.tar.gz"
  name "bwkp"
  desc "Transfer records between Bitwarden or Vaultwarden and KeePassXC"
  homepage "https://github.com/Neur0toxine/bwkp"

  depends_on macos: ">= :ventura"

  binary "bwkp"
end
