# Esquadro

A Go project generator inspired by JHipster. You describe your system in a
`.esq` file (entities, fields, relationships) and pick a set of **blueprints**
(template plugins). `esquadro gen` generates a complete Go project, and you can
regenerate it whenever the model changes.

```
app shop {
  module "github.com/example/shop"

  blueprint platform
  blueprint postgres
}

entity Product {
  name     string  required min(3)
  price    decimal required min(0)
  category -> Category (name)
}
```

> Work in progress. Not ready for use yet.

## Development

Requires Go and [Task](https://taskfile.dev).

```
task test       # run tests
task lint       # gofmt + go vet
task build      # binary in bin/esquadro
task run -- version
```
