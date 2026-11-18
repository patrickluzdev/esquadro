---
title: Files
description: File and image fields, kept in a folder, S3, Google Cloud Storage or Azure, sent straight to the store and read by signed links.
---

The [storage](/esquadro/blueprints/reference/storage/) blueprint keeps the
files of the model's `file` and `image` fields. `esquadro add storage` adds
it to a project.

```esquadro
app shop {
  blueprint storage
}

entity Product {
  photo  image required @storage(max "5MB")
  manual file  @storage(max "20MB", types [pdf], url_minutes 5)
}
```

## Where files go

`STORAGE_URL`, in each environment's `.env`, names the store:

| `STORAGE_URL` | The store |
|---|---|
| `file://./storage` | A folder: development's, by default |
| `s3://bucket?region=us-east-1` | S3 |
| `s3://bucket?endpoint=https://...&s3ForcePathStyle=true&region=auto` | What speaks S3: R2, MinIO, Spaces, B2 |
| `gs://bucket` | Google Cloud Storage, with `drivers [s3, gcs]` |
| `azblob://container` | Azure Blob Storage, with `drivers [s3, azure]` |
| `mem://` | In memory, for tests |

In development the store is a folder of the project (`storage/`, out of
git), which the app serves itself: the same flow as in production, with no
bucket, no Docker and no account anywhere. `STORAGE_PREFIX` starts every
file's key (`shop/production/`), to share a bucket.

To develop against S3 itself, `local s3` puts a local S3 in `compose.yaml`
(the Versity S3 Gateway, its bucket a folder of its volume) and points
development's `STORAGE_URL` at it. An S3 store that is no AWS account's own
(R2, MinIO and others) takes its keys in `STORAGE_ACCESS_KEY` and
`STORAGE_SECRET_KEY`; empty, the AWS SDK finds them (`AWS_ACCESS_KEY_ID`, a
role).

## Sending a file

A file goes from the browser straight to the store, never through the app:

1. `POST /storage/uploads` with `{entity, field, name, content_type,
   size}`: the API checks the field's rules (its size, its formats) and
   answers `{id, url, method, headers, expires_at}`.
2. A `PUT` of the bytes to `url`, with `headers`, before `expires_at`.
3. The record is created or changed with the file's id in its field:
   `{"photo": "01a1..."}`.

When the record is saved, in its transaction, the app checks the file: it
was asked for this field, by whoever saves it, is in the store at its size,
and is of the format it claims; its first bytes are read, so a "PNG" that is
a web page is refused (`invalid_type`). The TypeScript client does the
three steps in one, with progress:

```ts
const id = await api.uploads.send({ entity: "products", field: "photo", file, onProgress })
await api.products.patch(product.id, { photo: id }, { ifMatch: product })
```

A record answers its file as `{id, name, size, content_type, url}`: `url`
is a signed link to read it, which expires after the field's `url_minutes`.
An image of a safe format opens in the browser; any other file downloads
under its name (`inline` or `attachment` in `@storage` changes it). SVG,
which can run script, always downloads.

## No files left behind

A file replaced or cleared, or of a deleted record, is released, then
leaves the store; a file sent and never used is deleted after
`pending_hours`. With [jobs](/esquadro/blueprints/reference/jobs/) the
cleanup runs every `clean_every`; with a worker and no jobs, in the worker;
otherwise `<app> storage clean`, by cron.

A bucket must let browsers send to it: `<app> storage cors https://admin.shop.com`
prints the CORS rules to paste in the provider's console.

## Rules

| Where | What |
|---|---|
| The blueprint's options | `image_types` and `image_max` (png, jpeg, webp, gif, avif; 10MB), `file_types` and `file_max` (any; 25MB), `url_minutes`, `upload_minutes`, `pending_hours`, `clean_every`, `key_layout`, `check_content`, `drivers` |
| A field's `@storage` | `max "5MB"`, `types [pdf, docx]` (extensions or MIME types), `url_minutes 5`, `inline`, `attachment` |

A file field cannot be `unique`, have a `default`, `min`, `max` or
`pattern`, nor be a job's argument or a report's column.

## In the frontends

A [spa](/esquadro/frontends/) shows a file field as a field to drop or
choose a file in, checked for its size and format before it is sent, with
a preview and its progress, to replace or remove; an image in a list's
row, a file as a link to download it. A record's first image is its
picture, beside its name in lists and atop its page.
