# Contributing

1. Open an issue for user-visible behavior changes.
2. Run `make check` before submitting a pull request.
3. Keep public API changes additive and update `api/openapi.yaml` in the same commit.
4. Add focused tests for new behavior and preserve loopback-only defaults.

Use Conventional Commit subjects. Never commit `.env`, private keys, generated certificates, databases, or `node_modules`.
