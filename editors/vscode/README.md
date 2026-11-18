# Esquadro for VS Code

Support for [Esquadro](https://github.com/patrickluzdev/esquadro)'s `.esq` files:

- **Syntax highlighting** of models (`app`, `platform`, `server`, `package`, `enum`, `entity`, `job`, `report`)
  and blueprint manifests (`blueprint.esq`).
- **Errors while you type**, before saving: the ones `esquadro check` reports,
  across the whole project (an entity misspelled in another file, an option a
  blueprint does not have), with the same suggestions.
- **Format on save**, exactly as `esquadro fmt` does. Turn it off with
  `"[esquadro]": { "editor.formatOnSave": false }`.
- **The Esquadro panel**, in Source Control, beside git: what `esquadro gen`
  would do to each file of the project, grouped (to resolve, to merge into
  yours, to update, to create, to delete, yours kept), refreshed when the
  model or the lockfile changes. Click a file to see its difference; use the
  buttons to generate or to upgrade the project (in a terminal, with the
  notes of what changed).
- **Conflicts in three columns.** A conflict `esquadro gen` left in one of
  your files opens in the merge editor: the template as it was (base),
  yours, and the template now; the result below. Conflicts also show in
  Problems and the status bar, and saving a file still with markers warns.

## Requirements

The extension runs `esquadro lsp`, the language server built into esquadro,
and, for the panel, `esquadro gen -dry-run -json` (esquadro v0.8.0 or later).
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
