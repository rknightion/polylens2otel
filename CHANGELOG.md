# Changelog

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
