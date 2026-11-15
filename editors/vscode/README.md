# Esquadro for VS Code

Support for [Esquadro](https://github.com/patrickluzdev/esquadro)'s `.esq` files:

- **Syntax highlighting** of models (`app`, `package`, `enum`, `entity`) and
  blueprint manifests (`blueprint.esq`).
- **Errors while you type**, before saving: the ones `esquadro check` reports,
  across the whole project (an entity misspelled in another file, an option a
  blueprint does not have), with the same suggestions.
- **Format on save**, exactly as `esquadro fmt` does. Turn it off with
  `"[esquadro]": { "editor.formatOnSave": false }`.

## Requirements

The extension runs `esquadro lsp`, the language server built into esquadro.
Install esquadro and make sure `esquadro` is on your PATH, or set
`esquadro.path` to the command.

## Install from a .vsix

```
task editor:package                      # in the esquadro repository
code --install-extension editors/vscode/esquadro-0.1.0.vsix
```

## Other editors

Any editor with an LSP client can use the same server: run `esquadro lsp` for
files ending in `.esq` (it speaks LSP on stdin and stdout).
