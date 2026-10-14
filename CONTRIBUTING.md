# Contributing

Run `make check-all` before sending a change. It builds the code, runs the tests and runs the lints. [docs/development.md](docs/development.md) says how the code and its tests are laid out.

## Releases

- Changes collect at the top of `CHANGELOG.md`, under the next version marked `(unreleased)`.
- A version is tagged when a batch is worth upgrading for, not for each change.
- Breaking changes are grouped into one release where they can be, not shipped one per release.
