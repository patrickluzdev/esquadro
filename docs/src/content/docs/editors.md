---
title: Editors
description: Syntax highlighting, errors while you type and formatting for .esq files.
---

## VS Code

Install **Esquadro** from the
[Marketplace](https://marketplace.visualstudio.com/items?itemName=patrickluzdev.esquadro),
or from [Open VSX](https://open-vsx.org/extension/patrickluzdev/esquadro) in
VSCodium, Cursor and other editors that use it. It:

- colors `.esq` files, models and blueprint manifests;
- shows errors as you type, before saving, across the whole project, with the
  same messages and suggestions as `esquadro check`;
- formats on save, as `esquadro fmt` does. Turn it off with
  `"[esquadro]": { "editor.formatOnSave": false }`.

The extension runs `esquadro lsp`, so esquadro must be installed. If it is not
on the PATH, set `esquadro.path` to the command.

## Other editors

`esquadro lsp` is a language server: any editor with an LSP client can start
it for files ending in `.esq`. In Neovim, for example:

```lua
vim.filetype.add({ extension = { esq = "esquadro" } })
vim.lsp.config("esquadro", { cmd = { "esquadro", "lsp" }, filetypes = { "esquadro" } })
vim.lsp.enable("esquadro")
```

Blueprints from git are read from the cache only: if one has not been fetched
yet, the editor warns and asks you to run `esquadro gen` once.
