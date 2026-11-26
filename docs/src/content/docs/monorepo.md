---
title: In a monorepo
description: A project in a folder of a repository that holds more than it, such as a panel and a site written by hand.
---

A repository can hold more than the project: an API made by esquadro in
`api/`, beside a panel and a site you write yourself. Say where the project
is with the `path` option of the `git` blueprint:

```esquadro
blueprint git {
  path "api"
}
```

The project is everything in that folder, as before: its model in
`api/esquadro/`, its code, its migrations, its `Dockerfile`. What belongs to
the repository goes to its root:

| | At the root of the repository | In a project at the root |
|---|---|---|
| The repository | found at the root, or created there | in the project |
| The CI | `.github/workflows/api.yml`, named after the folder | `.github/workflows/ci.yml` |
| When the CI runs | changes in `api/` and to its workflow | every change |
| Release tags | `api/v1.2.3` | `v1.2.3` |

The workflow runs every command in `api/`, and builds the image from it.
A tag `api/v1.2.3` publishes the image as `1.2.3`, `1.2` and `latest`, and
deploys `v1.2.3` with `deploy`, as a tag `v1.2.3` does in a project at the
root. The other parts of the repository bring their own workflows and tags.

The CI's workflow is in the lockfile as `../.github/workflows/api.yml`:
esquadro writes outside the project only there, at the root of its
repository, and never into the project's own folder from above. Moving the
project, or taking the path away, moves the workflow as any generated file
that changes place: the old one is deleted when it is untouched.

## Not yet

- `branching`: its tasks, `CONTRIBUTING.md` and branch protection are the
  whole repository's, and esquadro refuses it with a path.
- A platform in a folder of its repository.
