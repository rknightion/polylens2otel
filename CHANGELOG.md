# Changelog

## [0.3.1](https://github.com/rknightion/polylens2otel/compare/v0.3.0...v0.3.1) (2026-09-05)


### Bug Fixes

* **auto-rc:** grant actions: read to the publish caller ([8e25dcb](https://github.com/rknightion/polylens2otel/commit/8e25dcb7df22c690161a8c20917c9e5d342b6dd4))
* **ci:** repin rknightion/.github refs to v1.9.7 so Renovate can track them ([3d7591b](https://github.com/rknightion/polylens2otel/commit/3d7591b475eb46b78b8dda06a94c47f0d8c06c7a))
* declare docs social card ([c65b757](https://github.com/rknightion/polylens2otel/commit/c65b757712c5d5010502575f1cd5817744ae07ac))
* **grafana-sync:** repoint the git add pathspec at grafana/ too ([eea1f75](https://github.com/rknightion/polylens2otel/commit/eea1f755c135a7f575adfbc05abb4ca855b46ee1))
* **grafana-sync:** write into the hub's grafana/ subtree ([e6f9b27](https://github.com/rknightion/polylens2otel/commit/e6f9b27478aebbb32ebfc095b81ec57236ccb6da))

## [0.3.0](https://github.com/rknightion/polylens2otel/compare/v0.2.0...v0.3.0) (2026-08-13)


### Features

* **config:** support phone-only startup without Lens ([ad36b32](https://github.com/rknightion/polylens2otel/commit/ad36b320f56167c88c11a8f0826e3c303b33f3da)), closes [#51](https://github.com/rknightion/polylens2otel/issues/51)
* **grafana:** add comprehensive v2 dashboard ([666a77a](https://github.com/rknightion/polylens2otel/commit/666a77a5687488bfbccd903f8d5a60be3cf9fd4e))
* **phone:** default to Lens policy authentication ([33c448b](https://github.com/rknightion/polylens2otel/commit/33c448b3afba5f4ba0a83167b7d474452ae89218)), closes [#51](https://github.com/rknightion/polylens2otel/issues/51) [#55](https://github.com/rknightion/polylens2otel/issues/55)
* **phone:** emit call and network telemetry ([bfc090f](https://github.com/rknightion/polylens2otel/commit/bfc090febd7a6ca4c41d6bd445636bd531ac95ac)), closes [#57](https://github.com/rknightion/polylens2otel/issues/57) [#58](https://github.com/rknightion/polylens2otel/issues/58)


### Bug Fixes

* **ci:** call GHCR pagination correctly ([7642e48](https://github.com/rknightion/polylens2otel/commit/7642e483e5ab7f5cb6aef3572da53e6a90fbdae7)), closes [#49](https://github.com/rknightion/polylens2otel/issues/49)
* **ci:** consume verified actionlint installer ([9c3bce4](https://github.com/rknightion/polylens2otel/commit/9c3bce4c305b9326c112fcd01e3157c46b67e7c5)), closes [#53](https://github.com/rknightion/polylens2otel/issues/53)
* **ci:** pin release installer versions ([b5e4225](https://github.com/rknightion/polylens2otel/commit/b5e42254a4583807ee45154b7b3c7d08966d14ba)), closes [#48](https://github.com/rknightion/polylens2otel/issues/48)
* **config:** document collector intervals ([af6fd5d](https://github.com/rknightion/polylens2otel/commit/af6fd5d3afc376a30fdbcf8a0333ab2fb0baa313)), closes [#61](https://github.com/rknightion/polylens2otel/issues/61)
* **grafana:** correct live dashboard queries ([d7f7208](https://github.com/rknightion/polylens2otel/commit/d7f7208914762f5ff822b7ed85e920df84f627f5)), closes [#67](https://github.com/rknightion/polylens2otel/issues/67)
* **grafana:** keep healthy uptime green ([ca10d89](https://github.com/rknightion/polylens2otel/commit/ca10d893678b1e581ad8a67cdc72426bf38c52c2)), closes [#67](https://github.com/rknightion/polylens2otel/issues/67)
* **grafana:** render collector traces and CDR rates ([823443c](https://github.com/rknightion/polylens2otel/commit/823443cc0466bb58e5bc9e2a095413ccd13e6786)), closes [#67](https://github.com/rknightion/polylens2otel/issues/67)
* **phone:** apply phone timezone to call logs ([8dd886d](https://github.com/rknightion/polylens2otel/commit/8dd886d24dc4ab5b07b8c3191a52af0fa002f056)), closes [#65](https://github.com/rknightion/polylens2otel/issues/65)
* **phone:** migrate call-log watermarks ([6539c7b](https://github.com/rknightion/polylens2otel/commit/6539c7bb0e76c46c6b3109bf6befb6203e55b183)), closes [#65](https://github.com/rknightion/polylens2otel/issues/65)
* **release:** verify Syft downloads ([902bf45](https://github.com/rknightion/polylens2otel/commit/902bf450a8f33b6dcd2b373d61567faf0a85c70d)), closes [#48](https://github.com/rknightion/polylens2otel/issues/48)
* **stream:** treat cancellation as clean shutdown ([f2f6882](https://github.com/rknightion/polylens2otel/commit/f2f6882cb9e5014d6c380f5d6bc4ecee3cd26faf)), closes [#56](https://github.com/rknightion/polylens2otel/issues/56)

## [0.2.0](https://github.com/rknightion/polylens2otel/compare/v0.1.0...v0.2.0) (2026-08-12)


### Features

* **phone:** resolve credentials from Lens policy ([d660a90](https://github.com/rknightion/polylens2otel/commit/d660a9092b6bc4ad9bb4ff11f9263da70f9a3d4a)), closes [#41](https://github.com/rknightion/polylens2otel/issues/41)


### Bug Fixes

* **release:** install Syft for binary SBOMs ([e98a182](https://github.com/rknightion/polylens2otel/commit/e98a1820c7caee7acc3a9d749ffc7abb81b24a71))
* **release:** resolve metadata from release tag ([25c6f22](https://github.com/rknightion/polylens2otel/commit/25c6f22b1a41e67c3eb4f235a5a4d681cee7bbee))

## 0.1.0 (2026-08-12)


### Features

* **cdr:** add checkpointed log collector ([43d766c](https://github.com/rknightion/polylens2otel/commit/43d766c86f76cd0f2239d17572ebbe0c8b67ddf9)), closes [#10](https://github.com/rknightion/polylens2otel/issues/10)
* **deploy:** add container and Helm packaging ([cf255d8](https://github.com/rknightion/polylens2otel/commit/cf255d8450b8db4c7121aa4e77b1e7351693a297)), closes [#6](https://github.com/rknightion/polylens2otel/issues/6)
* **grafana:** generate dashboards and alerts ([527a82c](https://github.com/rknightion/polylens2otel/commit/527a82ccd15414343fabadc2c3451ffe3efb393d)), closes [#14](https://github.com/rknightion/polylens2otel/issues/14)
* **lens:** add polling collectors ([66de137](https://github.com/rknightion/polylens2otel/commit/66de137699f850db634bdb0cb397ecbd26dc3b15)), closes [#8](https://github.com/rknightion/polylens2otel/issues/8)
* **lens:** add read-only GraphQL client ([09d87dd](https://github.com/rknightion/polylens2otel/commit/09d87dd4366cc9e550c9c9bd9b1146fc33c6b322)), closes [#3](https://github.com/rknightion/polylens2otel/issues/3)
* **phone:** add fleet-state collectors ([0f11d4d](https://github.com/rknightion/polylens2otel/commit/0f11d4d51d4db71827b27f4f7e3d4d57d7008a2b)), closes [#11](https://github.com/rknightion/polylens2otel/issues/11)
* **phone:** add identity-checked REST client ([f359e28](https://github.com/rknightion/polylens2otel/commit/f359e282fd0288474344d620a7c5a68fdbf1dc57)), closes [#4](https://github.com/rknightion/polylens2otel/issues/4)
* **phone:** add safe target resolution ([6ee257c](https://github.com/rknightion/polylens2otel/commit/6ee257c5d0a0fb042c9a7bcabf122355ba6f8a06)), closes [#12](https://github.com/rknightion/polylens2otel/issues/12)
* **runtime:** wire exporter integration ([b2fb09d](https://github.com/rknightion/polylens2otel/commit/b2fb09dfb5fe52bc6148fb806d4e51265981c8ba)), closes [#16](https://github.com/rknightion/polylens2otel/issues/16)
* **scaffold:** freeze exporter seams ([ffe7235](https://github.com/rknightion/polylens2otel/commit/ffe723526d34b1f5011e14f7281b509962bf1937)), closes [#2](https://github.com/rknightion/polylens2otel/issues/2)
* **selfobs:** add runtime instrumentation ([400b415](https://github.com/rknightion/polylens2otel/commit/400b4153c26ad4d350e3980a3109ff6f027d4f2a)), closes [#13](https://github.com/rknightion/polylens2otel/issues/13)
* **stream:** add named Lens subscription ([cf432ff](https://github.com/rknightion/polylens2otel/commit/cf432ff7e1a8f05494ec3008d9950fa8c4768705)), closes [#9](https://github.com/rknightion/polylens2otel/issues/9)


### Bug Fixes

* **build:** embed source metadata in images ([c1d9358](https://github.com/rknightion/polylens2otel/commit/c1d9358ff811bac3a2d933dff7be47e2a817cfd5)), closes [#34](https://github.com/rknightion/polylens2otel/issues/34)
* **ci:** close private workflow gaps ([afe18e7](https://github.com/rknightion/polylens2otel/commit/afe18e75a45b8e19c9147ca2d0658e8f23b30673)), closes [#5](https://github.com/rknightion/polylens2otel/issues/5)
* **ci:** make first-run gates phase-aware ([1740852](https://github.com/rknightion/polylens2otel/commit/17408523bf9e5bfa12114c2bcf4175a5b208fc93)), closes [#5](https://github.com/rknightion/polylens2otel/issues/5) [#6](https://github.com/rknightion/polylens2otel/issues/6)
* **ci:** support private first-run gates ([796485a](https://github.com/rknightion/polylens2otel/commit/796485ac79d127cbaaa302c1b492828416ea93d2)), closes [#5](https://github.com/rknightion/polylens2otel/issues/5)
* **deps:** update module github.com/coder/websocket to v1.8.15 ([#18](https://github.com/rknightion/polylens2otel/issues/18)) ([7e974b2](https://github.com/rknightion/polylens2otel/commit/7e974b2eb6f8f9583d9db8f1f22e10fdec4cbdc3))
* **deps:** update module github.com/grafana/pyroscope-go to v1.4.2 ([#19](https://github.com/rknightion/polylens2otel/issues/19)) ([78b9bcd](https://github.com/rknightion/polylens2otel/commit/78b9bcdc440a6745661020cee2d331342287594b))
* **hygiene:** distinguish source canaries from secrets ([fb6b411](https://github.com/rknightion/polylens2otel/commit/fb6b4116e3f8cb858690be266838f3eb44aa4678)), closes [#2](https://github.com/rknightion/polylens2otel/issues/2)
* **hygiene:** use documentation addresses in tests ([b5b5c9b](https://github.com/rknightion/polylens2otel/commit/b5b5c9b0205e70969d2b7cf07491e2db5060fc3b))
* **lens:** match live GraphQL variable types ([b81e8a6](https://github.com/rknightion/polylens2otel/commit/b81e8a6cd8fee6a920603073ada463b31d8a7e2c)), closes [#17](https://github.com/rknightion/polylens2otel/issues/17)
* **release:** force first release to v0.1.0 ([345fd53](https://github.com/rknightion/polylens2otel/commit/345fd5369b94827162bb6198f9ba237d1d52bec6))
* **release:** target initial v0.1.0 ([768be49](https://github.com/rknightion/polylens2otel/commit/768be49efc0dbee0f7965146cfe566ad0dd50981)), closes [#36](https://github.com/rknightion/polylens2otel/issues/36)
* **telemetry:** correct device series identity ([418edbe](https://github.com/rknightion/polylens2otel/commit/418edbe9d2ae99e515a7c693bbb803c7e2604cb0)), closes [#33](https://github.com/rknightion/polylens2otel/issues/33)

## Changelog

All notable changes to polylens2otel will be documented here.
