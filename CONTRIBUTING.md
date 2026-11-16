# Contributing

Thanks for helping. Issues and pull requests are welcome; for a large change,
open an issue first so we can agree on the approach.

## Setup

You need [Go](https://go.dev/dl), [Task](https://taskfile.dev) and Docker
(the end-to-end tests run Postgres in containers). For the VS Code extension
and the documentation site, Node.js.

```
task test          # unit and integration tests
task lint          # gofmt, go vet and gopls checks: any finding fails
task e2e           # a generated project through its whole life (Docker)
task editor:test   # the extension's grammar
task docs          # the documentation site, locally
```

Run `task lint test e2e` before opening a pull request; CI runs the same on
Linux and macOS. Windows is not supported yet.

## The VS Code extension

It lives in `editors/vscode/`. To try a change in your editor:

```
task editor:package
code --install-extension editors/vscode/esquadro-*.vsix
```

then "Developer: Reload Window". The version in its `package.json` does not
matter: a release stamps the tag's version on the extension it publishes.
Its `README.md` is the extension's page on the Marketplace and Open VSX:
write it for the people who install it.

## Conventions

- Commits follow [Conventional Commits](https://www.conventionalcommits.org):
  one line, in English, e.g. `feat(rest): add ETag support`.
- Code, messages and documentation are in English.
- Packages and files are organized by what they do, named by intent; no
  `utils`, `helpers`, `service` or `manager` names.
- Generated code must read as if a good Go developer wrote it by hand: no
  runtime magic, and `gopls check` clean.
- A change to a blueprint comes with a test of what it generates; the
  reference pages of the documentation are regenerated with `task docs:gen`.

## Blueprints

Built-in blueprints live in `blueprints/`, one folder each. The criteria for
adding one, or merging some into one, are in the documentation's "Writing a
blueprint" page.
