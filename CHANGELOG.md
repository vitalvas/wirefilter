# Changelog

## [0.3.0](https://github.com/vitalvas/wirefilter/compare/v0.2.0...v0.3.0) (2026-03-21)


### Features

* support negated duration literals (-30m, -2d4h) ([4a4859f](https://github.com/vitalvas/wirefilter/commit/4a4859fb5864c12dc5ca2e6e55cf91dac3434558))

## [0.2.0](https://github.com/vitalvas/wirefilter/compare/v0.1.0...v0.2.0) (2026-03-21)


### Features

* add DisableRegex feature flag, optimize TimeValue to zero-alloc int64, add fuzz and bench coverage ([5d78854](https://github.com/vitalvas/wirefilter/commit/5d78854037d773d9afec2ff736226b5c979d4cc2))
* add parser multi-error recovery and property-based testing ([87bdf1f](https://github.com/vitalvas/wirefilter/commit/87bdf1f18ce606e0e1a58b005d9b343e368bced8))


### Performance Improvements

* zero-alloc int ranges, stack-buffered arrays/args, switch-based function dispatch ([1268c60](https://github.com/vitalvas/wirefilter/commit/1268c60667c8326d1a8fa04ba1c567edee6e8815))

## [0.1.0](https://github.com/vitalvas/wirefilter/compare/v0.0.1...v0.1.0) (2026-03-19)


### Features

* add Export methods for schema and execution context audit logging ([ef60a95](https://github.com/vitalvas/wirefilter/commit/ef60a9536e281de64e5aed4f3e784582e406c146))
* add native time and duration type support with temporal arithmetic ([f28b2a7](https://github.com/vitalvas/wirefilter/commit/f28b2a765df42096a10999cc2affc1cd002a5179))
* add release-please ([95a688e](https://github.com/vitalvas/wirefilter/commit/95a688efa529315a10ee150ce3b4434a9254b1cc))
* extract code ([d5ee10f](https://github.com/vitalvas/wirefilter/commit/d5ee10f5f71d684886c560c80f9568d04ecb5726))


### Bug Fixes

* address code review findings across caching, validation, API safety, and docs ([351df77](https://github.com/vitalvas/wirefilter/commit/351df77633b26d9e77d7ed52181274fca81df008))
* go version ([9b575f5](https://github.com/vitalvas/wirefilter/commit/9b575f5a3ab596b3bfb822fa6a898c3308c546fe))
* improve fuzz and benchmark test coverage with correctness invariants ([2c4725d](https://github.com/vitalvas/wirefilter/commit/2c4725d424010bdba6816029fc61e4c2a48c25de))
* normalize IPv4-mapped IPv6 addresses per RFC 4291, increase test coverage to 98.7% ([ee206f7](https://github.com/vitalvas/wirefilter/commit/ee206f7a46fafcdcbfe67f7953ebb08dd87abe68))
