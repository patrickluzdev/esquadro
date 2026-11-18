# Canary: Promo do Duro

A project with the model of a real one (the Promo do Duro MVP) and the
changes its team makes to it. `TestCanaryUpgrades` (e2e) generates it with
the last release of esquadro, makes those changes, upgrades it with the
esquadro being tested and checks it builds and passes its tests, with no
conflict; then it adopts a feature of the new esquadro (`money`).

- `*.esq`: the model, in what the last release understands.
- `yours/`: files of the project's own, copied in after the first generation.

When a release makes the model use something new, move it here after the
release (the test generates with the last one).
