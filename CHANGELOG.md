# Changelog

## [0.5.1](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.5.0...v0.5.1) (2025-10-20)


### Bug Fixes

* cli version ([#35](https://github.com/external-secrets-inc/esi-pod-webhook/issues/35)) ([331f0ae](https://github.com/external-secrets-inc/esi-pod-webhook/commit/331f0aeea6aa9197575eb260c149be83b8433593))

## [0.5.0](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.6...v0.5.0) (2025-10-20)


### Features

* okta federation ([#33](https://github.com/external-secrets-inc/esi-pod-webhook/issues/33)) ([44b6110](https://github.com/external-secrets-inc/esi-pod-webhook/commit/44b611039042c4028e7735defb191c6c8651f5e3))

## [0.4.6](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.5...v0.4.6) (2025-09-29)


### Bug Fixes

* use public images for esi-cli ([#31](https://github.com/external-secrets-inc/esi-pod-webhook/issues/31)) ([e45ba15](https://github.com/external-secrets-inc/esi-pod-webhook/commit/e45ba152ebe1ef47f6a12fad6496b30a6801de4a))

## [0.4.5](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.4...v0.4.5) (2025-09-29)


### Bug Fixes

* use correct namespace on certificate dns ([#29](https://github.com/external-secrets-inc/esi-pod-webhook/issues/29)) ([0949ff6](https://github.com/external-secrets-inc/esi-pod-webhook/commit/0949ff6f3b63508e3f13118db18ab81673778d5c))

## [0.4.4](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.3...v0.4.4) (2025-09-29)


### Bug Fixes

* namespace override ([#27](https://github.com/external-secrets-inc/esi-pod-webhook/issues/27)) ([a1db69d](https://github.com/external-secrets-inc/esi-pod-webhook/commit/a1db69df3b4be843f4a07c9d3d542568ac26f755))

## [0.4.3](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.2...v0.4.3) (2025-09-29)


### Bug Fixes

* add public images ([#25](https://github.com/external-secrets-inc/esi-pod-webhook/issues/25)) ([8d9d622](https://github.com/external-secrets-inc/esi-pod-webhook/commit/8d9d622bf548dbdf89d76123e830b6e27a161b08))

## [0.4.2](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.1...v0.4.2) (2025-06-13)


### Bug Fixes

* update esi-cli version ([#23](https://github.com/external-secrets-inc/esi-pod-webhook/issues/23)) ([4680de6](https://github.com/external-secrets-inc/esi-pod-webhook/commit/4680de640de7790fbdfde729b3c17a8454afa8c3))

## [0.4.1](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.4.0...v0.4.1) (2025-06-12)


### Bug Fixes

* bump esi-cli version ([#20](https://github.com/external-secrets-inc/esi-pod-webhook/issues/20)) ([e87415a](https://github.com/external-secrets-inc/esi-pod-webhook/commit/e87415a48453acfcb067bc319f7a044eb4232379))

## [0.4.0](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.3.2...v0.4.0) (2025-05-30)


### Features

* annotations to flags ([#16](https://github.com/external-secrets-inc/esi-pod-webhook/issues/16)) ([ac2c9bf](https://github.com/external-secrets-inc/esi-pod-webhook/commit/ac2c9bf633c1a227627979084688ac807a7042cb))


### Bug Fixes

* add spiffe related annotations ([#19](https://github.com/external-secrets-inc/esi-pod-webhook/issues/19)) ([b585af3](https://github.com/external-secrets-inc/esi-pod-webhook/commit/b585af38948018da5ea11cb9e1f89b91609fd5d5))

## [0.3.2](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.3.1...v0.3.2) (2025-05-23)


### Bug Fixes

* volume  mounting only container.0 but patching all containers ([#14](https://github.com/external-secrets-inc/esi-pod-webhook/issues/14)) ([e723b1e](https://github.com/external-secrets-inc/esi-pod-webhook/commit/e723b1edcd55a6f86f435f94edbd7e5f00ac6461))

## [0.3.1](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.3.0...v0.3.1) (2025-05-23)


### Bug Fixes

* not use latest on esi-cli and esi-cli-init ([#12](https://github.com/external-secrets-inc/esi-pod-webhook/issues/12)) ([e5ba057](https://github.com/external-secrets-inc/esi-pod-webhook/commit/e5ba05720c5ac55890722d356c65d24971d3e03a))

## [0.3.0](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.2.1...v0.3.0) (2025-05-23)


### Features

* more annotation handling on webhook ([#10](https://github.com/external-secrets-inc/esi-pod-webhook/issues/10)) ([220d9cd](https://github.com/external-secrets-inc/esi-pod-webhook/commit/220d9cd33519751a918e8c47375576b92ab56e7b))

## [0.2.1](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.2.0...v0.2.1) (2025-05-22)


### Bug Fixes

* also parse args ([#8](https://github.com/external-secrets-inc/esi-pod-webhook/issues/8)) ([b04edb2](https://github.com/external-secrets-inc/esi-pod-webhook/commit/b04edb2f1e62f453783e63d23ead6eba64abf6ad))

## [0.2.0](https://github.com/external-secrets-inc/esi-pod-webhook/compare/v0.1.0...v0.2.0) (2025-05-22)


### Features

* adds imagepullsecrets annotations for webhook ([#6](https://github.com/external-secrets-inc/esi-pod-webhook/issues/6)) ([b9c4a5a](https://github.com/external-secrets-inc/esi-pod-webhook/commit/b9c4a5a8ae810713647e688d490edbc88b2f6d90))

## [0.1.0](https://github.com/external-secrets-inc/esi-pod-webhook/compare/0.0.1...v0.1.0) (2025-05-22)


### Features

* webhook using cli ([#5](https://github.com/external-secrets-inc/esi-pod-webhook/issues/5)) ([4714fc8](https://github.com/external-secrets-inc/esi-pod-webhook/commit/4714fc889928e578c3e2f7f47bbf1f134164fc52))
