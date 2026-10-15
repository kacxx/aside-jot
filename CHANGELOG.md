# Changelog

## [0.5.0](https://github.com/kacxx/aside-jot/compare/v0.4.0...v0.5.0) (2026-10-15)


### Features

* install the aside skill as a Claude Code plugin ([#117](https://github.com/kacxx/aside-jot/issues/117)) ([f22f2d8](https://github.com/kacxx/aside-jot/commit/f22f2d8ef1e62dab1d9646be577c166784e68bfe))

## [0.4.0](https://github.com/kacxx/aside-jot/compare/v0.3.0...v0.4.0) (2026-10-15)


### Features

* **skill:** offer a follow-up jot when closing a jot, keep done notes safe ([#104](https://github.com/kacxx/aside-jot/issues/104)) ([cffde9f](https://github.com/kacxx/aside-jot/commit/cffde9f22528c54f06dfacc6642f606153037493))
* **skill:** Windows follow-up form, partly done verdict, follow-ups in update all ([#106](https://github.com/kacxx/aside-jot/issues/106)) ([27ec14f](https://github.com/kacxx/aside-jot/commit/27ec14f3050ba6a9ecbc89ae45e743364de0c2e1))


### Bug Fixes

* strip a leading byte order mark from aside add stdin ([#107](https://github.com/kacxx/aside-jot/issues/107)) ([e636bd0](https://github.com/kacxx/aside-jot/commit/e636bd04fa67a1aa08ff4e705a1c8b4477d2d4e2))

## [0.3.0](https://github.com/kacxx/aside-jot/compare/v0.2.0...v0.3.0) (2026-10-14)


### Features

* add a read-only activity tool to the MCP server ([#89](https://github.com/kacxx/aside-jot/issues/89)) ([a16b769](https://github.com/kacxx/aside-jot/commit/a16b7695bdaf9591915091d91f629bfaf3e3b006))
* add aside activity to show what a jot's session did after it ([#88](https://github.com/kacxx/aside-jot/issues/88)) ([3e019cd](https://github.com/kacxx/aside-jot/commit/3e019cd443fd28feae73a161d881852f8182f1fc))
* store the transcript path of Codex jots ([#87](https://github.com/kacxx/aside-jot/issues/87)) ([51497f6](https://github.com/kacxx/aside-jot/commit/51497f6e8428dc3ea0126e9eb7cd610a6ffe7e8a)), closes [#83](https://github.com/kacxx/aside-jot/issues/83)
* teach the aside skill an update-all routine with a hard limit ([#91](https://github.com/kacxx/aside-jot/issues/91)) ([2898ff4](https://github.com/kacxx/aside-jot/commit/2898ff457c3da31d9438712aba6944154967d78e))
* teach the aside skill to update a jot from its session ([#90](https://github.com/kacxx/aside-jot/issues/90)) ([6ddfcaa](https://github.com/kacxx/aside-jot/commit/6ddfcaa2e85fbe0cbb3f9458b98b5a9420ace656))


### Bug Fixes

* **release:** binaries print the version tag with its v ([#76](https://github.com/kacxx/aside-jot/issues/76)) ([8dec86b](https://github.com/kacxx/aside-jot/commit/8dec86b559d12b5a001c60b06807fe993445ed18))
* Windows resume command, open hint in sessions, clearer temp-dir setup error ([#80](https://github.com/kacxx/aside-jot/issues/80)) ([317c721](https://github.com/kacxx/aside-jot/commit/317c7212c67a47dd67a77f6d5f26e13724b3a163))

## [0.2.0](https://github.com/kacxx/aside-jot/compare/v0.1.1...v0.2.0) (2026-10-11)


### Features

* add an optional agent skill for reading jots ([#72](https://github.com/kacxx/aside-jot/issues/72)) ([f0fbae1](https://github.com/kacxx/aside-jot/commit/f0fbae12eaabfdc89f625787cf26632250fae482))

## [0.1.1](https://github.com/kacxx/aside-jot/compare/v0.1.0...v0.1.1) (2026-10-08)


### Bug Fixes

* **mcp:** quote jot text exactly and copy commands with the full path ([#70](https://github.com/kacxx/aside-jot/issues/70)) ([68244d7](https://github.com/kacxx/aside-jot/commit/68244d737870448766f4fc2d8cb383776105b29b))

## 0.1.0 (2026-10-08)


### Features

* add sessions tool and older_than_days to the MCP server ([#62](https://github.com/kacxx/aside-jot/issues/62)) ([8e4b5d9](https://github.com/kacxx/aside-jot/commit/8e4b5d9bc9bbdf8019062bc9686f93d4bf91975a))
