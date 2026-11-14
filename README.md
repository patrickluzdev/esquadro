# Esquadro

Gerador de projetos Go inspirado no JHipster. Você descreve o sistema num
arquivo `.esq` (entidades, campos, relações) e escolhe as **plantas** (plugins
de templates). O `esquadro gen` gera um projeto Go completo, que pode ser
regenerado sempre que o modelo mudar.

```
app loja {
  module "github.com/exemplo/loja"
  lang pt

  planta base
  planta postgres
}

entity Produto {
  nome      string  required min(3)
  preco     decimal required min(0)
  categoria -> Categoria (nome)
}
```

> Em desenvolvimento. Ainda não está pronto para uso.

## Desenvolvimento

Requer Go e [Task](https://taskfile.dev).

```
task test       # testes
task lint       # gofmt + go vet
task build      # binário em bin/esquadro
task run -- version
```
