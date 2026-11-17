# Esquadro for VS Code

Support for [Esquadro](https://github.com/patrickluzdev/esquadro)'s `.esq` files:

- **Syntax highlighting** of models (`app`, `package`, `enum`, `entity`, `job`, `report`)
  and blueprint manifests (`blueprint.esq`).
- **Errors while you type**, before saving: the ones `esquadro check` reports,
  across the whole project (an entity misspelled in another file, an option a
  blueprint does not have), with the same suggestions.
- **Format on save**, exactly as `esquadro fmt` does. Turn it off with
  `"[esquadro]": { "editor.formatOnSave": false }`.

## Requirements

The extension runs `esquadro lsp`, the language server built into esquadro.
[Install esquadro](https://patrickluzdev.github.io/esquadro/getting-started/)
and make sure `esquadro` is on your PATH, or set `esquadro.path` to the
command.

## Install

In VS Code, search the Extensions view for **Esquadro**, or run:

```
code --install-extension patrickluzdev.esquadro
```

VSCodium, Cursor and other editors that use [Open VSX](https://open-vsx.org/extension/patrickluzdev/esquadro)
find it there, under the same name.

Extensions install into the current profile. If a folder opens in another
profile (its name shows in the window's title bar), install there too:
`code --profile "<profile>" --install-extension patrickluzdev.esquadro`.

## Other editors

Any editor with an LSP client can use the same server: run `esquadro lsp` for
files ending in `.esq` (it speaks LSP on stdin and stdout).
