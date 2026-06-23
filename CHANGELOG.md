# Changelog

## [0.6.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.5.0...agent-tunnel-v0.6.0) (2026-06-23)


### Features

* **cli:** distribute agent-tunnel CLI via agentics.dk install ([895d703](https://github.com/pksorensen/pks-agent-tunnel/commit/895d70393dc4147d4f23c19992466802636a4f3b))


### Bug Fixes

* **router:** trust X-Forwarded-Proto so https redirects survive TLS edge ([7ac93e5](https://github.com/pksorensen/pks-agent-tunnel/commit/7ac93e52c89cb5844d03825386a22476c7e663f6))

## [0.5.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.4.0...agent-tunnel-v0.5.0) (2026-06-09)


### Features

* **tunnel:** expose per-slot resource so QR codes attach to the URL ([9e0a76d](https://github.com/pksorensen/pks-agent-tunnel/commit/9e0a76d9163fd16f13156e9f9b04e6051b9fda20))

## [0.4.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.3.2...agent-tunnel-v0.4.0) (2026-05-28)


### Features

* **aspire:** WithPublicUrlOverride for proxied tunnel deployments ([caa6bb6](https://github.com/pksorensen/pks-agent-tunnel/commit/caa6bb6cf94b900cca3f7e7db42113e0baa651f4))

## [0.3.2](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.3.1...agent-tunnel-v0.3.2) (2026-05-20)


### Bug Fixes

* **router:** set X-Forwarded-Proto/Host so upstream redirects keep https ([d6c6caf](https://github.com/pksorensen/pks-agent-tunnel/commit/d6c6caf52262fdcfb262530a3b393a77b222fbab))

## [0.3.1](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.3.0...agent-tunnel-v0.3.1) (2026-05-20)


### Bug Fixes

* **ci:** bump build image to golang:1.25-alpine ([7ca86dd](https://github.com/pksorensen/pks-agent-tunnel/commit/7ca86ddc0809aeedb52ec1348eb559c513bd5627))

## [0.3.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.2.0...agent-tunnel-v0.3.0) (2026-05-20)


### Features

* **aspire:** nested slot resources + clickable URLs in dashboard ([8dd4756](https://github.com/pksorensen/pks-agent-tunnel/commit/8dd47565e28be66bfc747f1bc1a02106a4e6fafc))
* **server:** native Let's Encrypt via certmagic + Cloudflare DNS-01 ([16ec97d](https://github.com/pksorensen/pks-agent-tunnel/commit/16ec97d5c49617d87df028b3644f64e9f51d96dc))

## [0.2.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.5...agent-tunnel-v0.2.0) (2026-05-20)


### Features

* **aspire:** WithServer(url) for pointing at a remote tunnel server ([1649680](https://github.com/pksorensen/pks-agent-tunnel/commit/1649680b086bf07635d083b6d3933728dfd14870))
* **server:** PUBLIC_HTTP_PORT for host-vs-container port mismatch ([a57a3a5](https://github.com/pksorensen/pks-agent-tunnel/commit/a57a3a5b8e1c523e62a823825cfba9f01081cd24))

## [0.1.5](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.4...agent-tunnel-v0.1.5) (2026-05-18)


### Bug Fixes

* **devcontainer:** mirror pks-agent-ftp so PKS runner spawns dockerd ([8b29ab1](https://github.com/pksorensen/pks-agent-tunnel/commit/8b29ab1cf59e01bed0530eb413a7e3150979a725))

## [0.1.4](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.3...agent-tunnel-v0.1.4) (2026-05-16)


### Bug Fixes

* **ci:** start dockerd before docker build on the pks runner ([fe1432d](https://github.com/pksorensen/pks-agent-tunnel/commit/fe1432db5091921f9b61fff666e1563747dd690e))

## [0.1.3](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.2...agent-tunnel-v0.1.3) (2026-05-16)


### Reverts

* **ci:** back to pks self-hosted runner + registry.kjeldager.io ([849518e](https://github.com/pksorensen/pks-agent-tunnel/commit/849518e04046084c3bd8280f5ca9db2ba0577cc2))

## [0.1.2](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.1...agent-tunnel-v0.1.2) (2026-05-16)


### Bug Fixes

* **ci:** push to GHCR on ubuntu-latest; kjeldager registry now opt-in ([4623153](https://github.com/pksorensen/pks-agent-tunnel/commit/4623153c40853acb72f108c3137c4fcf10b9fd6d))

## [0.1.1](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.1.0...agent-tunnel-v0.1.1) (2026-05-16)


### Bug Fixes

* **ci:** start dockerd via docker-init.sh before image build ([b17269b](https://github.com/pksorensen/pks-agent-tunnel/commit/b17269bfc7d10aa55292fb7552e1d5918f0b48cf))

## [0.1.0](https://github.com/pksorensen/pks-agent-tunnel/compare/agent-tunnel-v0.0.1...agent-tunnel-v0.1.0) (2026-05-16)


### Features

* initial v0.1 — server, CLI, and Aspire drop-in extension ([9b07992](https://github.com/pksorensen/pks-agent-tunnel/commit/9b079924ad27e33c30fcc463a8d743cb73e94b55))

## [0.0.1] - 2026-05-16

Initial scaffold.
