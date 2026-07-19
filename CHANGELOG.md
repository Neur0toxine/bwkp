# Changelog

## [0.1.1](https://github.com/Neur0toxine/bwkp/compare/v0.1.1...v0.1.1) (2026-07-19)


### Bug Fixes

* add commonly used ~/.local/bin to install.sh ([20fb3be](https://github.com/Neur0toxine/bwkp/commit/20fb3be76d8d29a01cd236a967b3b9ec0791d306))
* Termux compat with install.sh ([638c308](https://github.com/Neur0toxine/bwkp/commit/638c308a95c0a0878a8fe7007fe86c2b15b7b0a7))

## [0.1.1](https://github.com/Neur0toxine/bwkp/compare/v0.1.0...v0.1.1) (2026-07-19)


### Features

* add detailed command help ([bd370fa](https://github.com/Neur0toxine/bwkp/commit/bd370fad21af2a0ffe66576594e69f36c66ef41b))
* add macOS Homebrew installation ([fb13ffb](https://github.com/Neur0toxine/bwkp/commit/fb13ffbe6c357260b255d038e5ae4172cb7aed2c))
* add Termux Android builds ([bc74a7e](https://github.com/Neur0toxine/bwkp/commit/bc74a7e91b6a82e0d85ffc817f45a91215145275))
* allow lossy convertations via flag ([6771350](https://github.com/Neur0toxine/bwkp/commit/677135031da7134d01905f548bbf4078a88ad9a2))
* import into bitwarden from keepass ([6089551](https://github.com/Neur0toxine/bwkp/commit/60895510d1ab1785d28648cae6367044cb98746f))
* progress bars ([14fd378](https://github.com/Neur0toxine/bwkp/commit/14fd3784c9577f459328952c08ccb6a77aa61b26))
* windows build ([354cfbd](https://github.com/Neur0toxine/bwkp/commit/354cfbdc8e1401395c71bf91c5fcb553e52d3409))


### Bug Fixes

* address vet and lint findings ([bd00135](https://github.com/Neur0toxine/bwkp/commit/bd001358a89836480d78bc569c3a3cad5ad38a16))
* align Windows x86 toolchain runtime ([9ba7abd](https://github.com/Neur0toxine/bwkp/commit/9ba7abdb3db76746a1ec27a9ff4275061f2253f1))
* allow Termux CI cache executables ([ef76563](https://github.com/Neur0toxine/bwkp/commit/ef7656358fd6861514c2355af50cdc9862981905))
* complete static cross-platform links ([420de95](https://github.com/Neur0toxine/bwkp/commit/420de95bdee64af3ea83e7a598ec01dcbe97ef4c))
* complete Windows x86 compiler sysroot ([000a486](https://github.com/Neur0toxine/bwkp/commit/000a48670fbc3b8022b3f6173fcef4402c616b80))
* configure Botan for Windows targets ([f0eec88](https://github.com/Neur0toxine/bwkp/commit/f0eec887bef360b40c4c38a40672ffe16d76456e))
* embed KeePassXC platform builds ([d2b001d](https://github.com/Neur0toxine/bwkp/commit/d2b001d64b61453f7d7edc95c60629b2913a0da8))
* enforce native build parallelism ([6df9e80](https://github.com/Neur0toxine/bwkp/commit/6df9e805e06e9c35b3cf4b70dbc6404adc9595e1))
* expose static dependencies to cross builds ([8e98162](https://github.com/Neur0toxine/bwkp/commit/8e98162d49c7d6072934c1b6ee98a8aeb5596090))
* fake converter progress ([9a49dd4](https://github.com/Neur0toxine/bwkp/commit/9a49dd4bd1a2ff02162a1485ee54e11198b69549))
* finish cross-platform static builds ([327596d](https://github.com/Neur0toxine/bwkp/commit/327596d872a410c4f068ae5fa85c5a36c6cdbf8f))
* first release will be 0.1.1 ([bca3e2c](https://github.com/Neur0toxine/bwkp/commit/bca3e2ca1f28d72223abb5b83badaa239b66b009))
* install MinGW Qt mkspecs ([27dc0c2](https://github.com/Neur0toxine/bwkp/commit/27dc0c25bfd86c4bf4364cb885a5a276ac89c2cf))
* install only required MinGW Qt artifacts ([5fcdca8](https://github.com/Neur0toxine/bwkp/commit/5fcdca8ddc38521577aeee6f65c13be013e72c1b))
* invoke static builds through host shells ([0886bb7](https://github.com/Neur0toxine/bwkp/commit/0886bb74871c578ae9fd1b4c1174cb45e35b9bcb))
* link macOS screen capture framework ([c80759d](https://github.com/Neur0toxine/bwkp/commit/c80759d5e14d0bd3199f079a9b1cf6a5099d9e1d))
* link Qt explicitly on Windows ([7043f88](https://github.com/Neur0toxine/bwkp/commit/7043f8831e13eff505c415abe96cefb3ebd2da4c))
* link static Qt against macOS frameworks ([7afa859](https://github.com/Neur0toxine/bwkp/commit/7afa8599989e69ded7158fbe1b3de855c11a00af))
* link Windows power GUIDs ([12c296d](https://github.com/Neur0toxine/bwkp/commit/12c296d27420dab208d535a9887b4ad9bcf58b72))
* local builds ([5406210](https://github.com/Neur0toxine/bwkp/commit/54062106b7b55b74030c699e9b5896c7d47b5d2c))
* make e2e database writable ([bc06ce2](https://github.com/Neur0toxine/bwkp/commit/bc06ce211e378207c0eefc0271c18533373bd850))
* make Termux CI cache writable ([0f67956](https://github.com/Neur0toxine/bwkp/commit/0f67956d67c0f94e43a5e6bc15b1a8e7e11ecc8b))
* normalize static dependency build environments ([8d81303](https://github.com/Neur0toxine/bwkp/commit/8d813038c0c3c406030c58787e56bf9f0741df67))
* provide Windows x86 linker sysroot ([96d76e1](https://github.com/Neur0toxine/bwkp/commit/96d76e177a40897d1fd2b7647c475a6ebceec297))
* provide x86 compiler sysroot ([6ddb182](https://github.com/Neur0toxine/bwkp/commit/6ddb18236181aeab29c4507c6a6df1043e739458))
* repair release CI setup ([c68d5fd](https://github.com/Neur0toxine/bwkp/commit/c68d5fdc00a1d66dd34ceee863d00dbc269aeddd))
* repair Windows native builds ([5edf882](https://github.com/Neur0toxine/bwkp/commit/5edf882e810f34857cab40fdc27722bd73d12cb1))
* repair Windows native toolchains ([37bcecb](https://github.com/Neur0toxine/bwkp/commit/37bcecbec95367e5cd388f9357716f35a5c9421f))
* skip MinGW Qt pkg-config ([b335694](https://github.com/Neur0toxine/bwkp/commit/b335694e0ed0f8de03fe6e832e22b404186a61e5))
* skip unused MinGW pkg-config metadata ([d71f394](https://github.com/Neur0toxine/bwkp/commit/d71f3949e70b1cbdb49745dee1b288d322bf646f))
* stabilize native CI builds ([88b7d34](https://github.com/Neur0toxine/bwkp/commit/88b7d3499c11db55cffcd65e0da013b4cd91a540))
* stabilize progress rendering ([7526f93](https://github.com/Neur0toxine/bwkp/commit/7526f937c945ffc962d37be3dad13ff1864b7fde))
* support macOS static dependency builds ([0fcda8d](https://github.com/Neur0toxine/bwkp/commit/0fcda8d1013377166468bc52d1b363c55067d8bf))
* usage and version output ([a4f6a1a](https://github.com/Neur0toxine/bwkp/commit/a4f6a1af2a95746845b02fc985c1031566d96232))
* use official user-agent and headers for attachments ([381dbc8](https://github.com/Neur0toxine/bwkp/commit/381dbc891bcae4061ac8448cd15a56a84cbea768))
