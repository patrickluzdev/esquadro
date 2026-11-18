---
title: Lists
description: Page, order, filter and search the API's lists, as the model declares.
---

Every entity's list (`GET /api/v1/products`) pages, orders, filters and
searches as its model says. The newest records come first unless the request
asks otherwise.

## Declaring them

What a list filters, orders and searches by is declared on each field, with
the [store](/esquadro/blueprints/reference/store/) blueprint: only what you
declare is open to the API, and each gets the index it needs in the
migrations.

```esquadro
entity Product {
  name        string  required @store(search, sort)
  description text    @store(search)
  price       decimal required @store(filter, sort)
  status      Status  default(ACTIVE) @store(filter)
  category    > Category required @store(filter)
}
```

- `filter`: by a value; numbers and dates also by a range (`_min`, `_max`,
  inclusive); an enum by a list of values. Relations filter by the related
  record's id. A filter's parameter cannot be one of the list's own
  (`page`, `per_page`, `limit`, `after`, `sort`, `q`, `fields`, `include`),
  nor another filter's (a field `price_min` beside `price` filtered by
  range): `esquadro check` says so at the field.
- `sort`: the list may be ordered by it, either way. Only a required field
  (or one with a default) may be: a page ordered by a field that may be empty
  would skip the records without it.
- `search`: its text is searched with `q`, PostgreSQL's full-text search.
  The `store` blueprint's `search_language` option (`simple` by default;
  `portuguese`, `english`...) decides how words are reduced to their root.

Every list may also filter by creation: `created_at_min`, `created_at_max`.

## Asking

```sh
GET /api/v1/products?status=ACTIVE,INACTIVE&price_min=10&price_max=50&sort=-price
GET /api/v1/products?category_id=0198…&q="board game" -chess
```

- `sort` is `created_at` or a field declared `sort`, with `-` for descending;
  the default is `-created_at`, the newest first. With `q`, the default is
  `relevance`, the best matches first.
- A value that is not one (`price_min=abc`) answers `422 invalid_fields`; an
  order the list does not take, `400`.

## Pages

Two ways of paging, both on by default:

- **By cursor:** `limit` per page (25 by default, at most 100); the answer's
  `next` is the cursor of the next page, absent on the last one: pass it as
  `after`. Pages stay right while records are added, which suits infinite
  scrolling and large tables. A cursor belongs to its order: another `sort`
  with it answers `400`.
- **By page:** `page` (from 1) and `per_page`; the answer has `page`,
  `per_page`, `total` and `total_pages`, for a table with page numbers.
  `total` counts the records on every request.

With both on, `page` (or `per_page`) asks for pages by number; otherwise the
list pages by cursor. Choose per entity, or for the project with the `rest`
blueprint's `pagination` option:

```esquadro
entity Category {
  name string required
  @rest(pagination [])          // every one at once
}

entity Order {
  ...
  @rest(pagination [cursor])    // by cursor only
}
```

A list without pagination answers every record, up to `unpaged_max` (1000
by default, per entity or for the project). Past it, it answers
`422 too_many_records` rather than some of them as if they were all: turn
pagination on for it.

## In your code

The module's use case takes the same query, without the API (`c` is the
catalog module):

```go
page, err := c.ListProducts(ctx, catalog.ProductQuery{
	Filter: catalog.ProductFilter{PriceMax: &max, Status: []catalog.Status{catalog.StatusActive}},
	Search: "board game",
	Sort:   "-price",
	Limit:  10,
})
```

[Policies](/esquadro/security/#policies) apply to the API's lists: filters,
search and `total` only count what the person may see.
