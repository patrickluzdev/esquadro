---
title: Branching
description: The repository's branching model, from GitFlow to trunk-based, with tasks that run it.
---

The [branching](/esquadro/blueprints/reference/branching/) blueprint sets the
repository up for a branching model, GitFlow by default:

```sh
esquadro add branching
```

```esquadro
blueprint branching {
  model "gitflow"   // gitflow | github_flow | gitlab_flow | trunk | release_flow
}
```

It creates the model's long-lived branches after the first commit (`develop`
for GitFlow), writes tasks that run the model with plain git, runs the CI on
the model's branches, and explains the model in `CONTRIBUTING.md`, a file of
yours. With the `git` blueprint, a new repository starts on `main` (the
`main` option), whatever git's own default is.

## The models

| Model | How it works | Tasks |
|---|---|---|
| `gitflow` | `main` is production, `develop` integrates; `feature/*` and `bugfix/*` go into `develop`, `release/*` and `hotfix/*` into `main` (tagged) and `develop` | `feature:start/finish`, `bugfix:start/finish`, `release:start/finish`, `hotfix:start/finish` |
| `github_flow` | only `main`, always deployable; short branches merged by pull request | `feature:start` |
| `gitlab_flow` | `main`, then environment branches (`staging`, `production`) code is promoted into, in order | `feature:start`, `promote -- <environment>` |
| `trunk` | everyone commits to `main`, small and often, with feature flags | `sync` |
| `release_flow` | `main`, and a `releases/X.Y` branch cut for each version; fixes are cherry-picked into them | `feature:start`, `release:cut -- <X.Y>`, `release:pick -- <commit> <X.Y>` |

## GitFlow, step by step

```sh
task feature:start -- login     # feature/login, from develop
# ... commits ...
task feature:finish             # merged into develop, the branch deleted
task release:start -- 1.2.0     # release/1.2.0, from develop
task release:finish             # into main, tagged v1.2.0, and back into develop
git push origin main develop v1.2.0
```

The tasks check before they change anything (a clean tree, the right
branch, a version as `X.Y.Z`) and never push: each one ends with what to
send. They are a shell script, `scripts/branching.sh`, that `task` runs. The
git-flow extension works too: its configuration is written for it.

## The CI

With `github_actions`, the CI runs on pushes to the model's long-lived
branches (`main` and `develop`, for GitFlow) and tags, and on pull requests,
which cover the other branches. For GitFlow, a pull request whose branch
does not follow the model (a `feature/*` into `main`) fails with a message
that says why.

## Protecting the branches

```sh
task github:protect
```

asks GitHub, with the [gh](https://cli.github.com) CLI and your account, to
require a pull request and a green CI to change the long-lived branches. It
runs only when you run it.
