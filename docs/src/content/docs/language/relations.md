---
title: Relations
description: Many-to-one, one-to-many, one-to-one and many-to-many, DBML style.
sidebar:
  order: 3
---

Relations are fields whose type is another entity, with a sign borrowed from
[DBML](https://dbml.dbdiagram.io):

| Sign | Relation | Stored as |
|---|---|---|
| `>` | many-to-one | a key on this entity (`category_id`) |
| `<` | one-to-many: the other side of a `>` | nothing new |
| `-` | one-to-one | a unique key on this entity |
| `<>` | many-to-many | a join table |

```esquadro
entity Product {
  category > Category (name) required on_delete(cascade)
  profile  - Profile
  tags     <> Tag
}

entity Category {
  products < Product
}
```

- `required` makes the key mandatory.
- `on_delete(cascade | restrict | set_null)` says what deleting the target does
  to this side's records; by default a required key restricts and an optional
  one is set to null.
- `(name)` names the target's field that shows it in lists and pickers.
- `<` is optional: within a module, the other side of every `>` is there
  anyway (`Category.products`). Write it to choose its name, and, when an
  entity points at another more than once, to say which: `< Product.backup`.

## What gets generated

With the `store` and `rest` blueprints, a many-to-many relation gets use cases
and operations to add, remove and list: `PUT` and `DELETE`
`/products/{id}/tags/{tagId}`, `GET /products/{id}/tags`, and the other side,
`GET /tags/{id}/products`.

## To the same entity

An entity may point at itself: comments that answer comments, categories
within categories, people and their manager.

```esquadro
entity Comment {
  body    text required max(2000)
  offer   > Offer required on_delete(cascade) @store(filter)
  parent  > Comment on_delete(cascade) @store(filter)   // the one it answers
  replies < Comment                                     // the ones that answer it
}
```

- The key (`parent_id`) points at the same table, with its `on_delete`:
  here deleting a comment deletes its replies, and their replies. With
  `@store(soft_delete)`, a comment goes to the trash with its replies, and
  they come back with it.
- `include=parent` answers a comment with the one it answers;
  `GET /api/v1/comments?parent_id=<id>` lists the replies of one, and
  `parent_id=null` the comments that answer none, the top ones
  (see [Lists](/esquadro/lists/)). In Go, `CommentFilter{ParentID: id}` and
  `CommentFilter{NoParent: true}`.
- In the admin panel, the parent is picked among the other comments, and a
  comment's page lists its replies. On a site,
  `@site(page, relations [replies])` shows them on a comment's page.
- How deep replies go is the project's: esquadro does not limit it. To keep
  one level, as most sites do, a rule makes a reply to a reply answer the
  comment at the top:

```go
func (CommentRules) BeforeCreate(ctx context.Context, m *Catalog, n *NewComment) error {
	if n.ParentID == uuid.Nil() {
		return nil
	}
	parent, err := m.ReadComment(ctx, n.ParentID)
	if err != nil {
		return err
	}
	if parent.ParentID != uuid.Nil() {
		n.ParentID = parent.ParentID
	}
	return nil
}
```

Unnamed, the other side of a relation to the same entity is named after
both (`comments_by_parent`): write the `<` to choose a better name.

## Across modules

An entity may point at an entity of another module with `>`, `-` or `<>`,
written by its name. Modules do not share tables: no foreign key is created
across modules, and the other module does not get the other side (a `<`
across modules is an error). A many-to-many relation across modules lists IDs.
