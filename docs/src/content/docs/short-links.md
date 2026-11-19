---
title: Short links
description: "A link shortener from the model: a site's addresses that redirect by a record's code, with where each visit goes and what is kept of it in the entity's rules."
---

A short link, `promo.com/aB3xY`, is an address that sends somewhere else
and counts who went. There is no shortener blueprint: what changes from one
app to another (what is counted, when a link ends, where it then goes) is
the app's own. What is the same is **an address per record that
redirects**, and the public site does that, as it gives records pages:

```esquadro
/// A short link of an offer for one group.
entity Link {
  code        string unique min(4) max(12)
  target_url  string required
  offer       > Offer required on_delete(cascade)
  @site(redirect, by code, to target_url)
}

/// A click on a link.
entity Click {
  occurred_at time   default(now)
  bot         bool   default(false)
  visitor     string
  link        > Link required on_delete(cascade)
}
```

`@site(redirect, by code, to target_url)` makes `/<code>` on the site send
to the link's `target_url` (`302`, never cached, so every visit comes by).
With `path "l"`, the addresses are `/l/<code>`. `by` is a unique string
field; `to`, the field of where it sends. An entity either redirects or has
pages on a site, and one at most redirects at a site's root.

A code no link has is the site's 404. A link preview (WhatsApp, Telegram,
a social network) is sent on too, so that it shows what the link leads to;
the visit says it is a bot.

## A page on the way

With `wait`, a person sees where they are going before they go:

```esquadro
  @site(redirect, by code, to target_url, wait 3)
```

`/<code>` then answers a page in the site's layout that says *Taking you to
shopee.com.br* (the destination's host), counts the seconds down with a
bar, and goes on by itself; *Go now* leaves at once. Without JavaScript,
the page's refresh goes on all the same. `wait` is 1 to 10 seconds, and
only with `redirect`.

Bots, a link's preview among them, still get the `302`, so that the preview
shows the destination; an address of the site (`/offers/1`, from
`BeforeRedirect`) is sent on at once too. Neither the page nor the `302` is
ever cached.

The page is yours: `link_redirect.templ`, written once, with
`LinkRedirectPage`, which receives a `LinkRedirect` (the `Page`, the
`Record`, `To`, its `Host` and the `Wait`). Its words are the site's
(`redirectTitle`, `redirectWait`, `redirectGo`), changed in `messages.go`.

## What each visit does: the entity's rules

The redirect calls two methods of `LinkRules`, in `link_rules.go`, yours,
beside `BeforeCreate` and the others. Generated, they do nothing: a link
sends to its `target_url`, and nothing is kept. Write them to decide:

```go
// A link without a code gets one made up.
func (LinkRules) BeforeCreate(ctx context.Context, m *Deals, n *NewLink) error {
	if n.Code == "" {
		n.Code = strings.ToLower(rand.Text()[:7])
	}
	return nil
}

// Where a visit goes: an offer that ended sends to its page on the site.
func (LinkRules) BeforeRedirect(ctx context.Context, m *Deals, l Link, v Visit) (string, error) {
	offer, err := m.ReadOffer(ctx, l.OfferID)
	if err != nil {
		return "", err
	}
	if offer.Status == OfferStatusExpired {
		return "/offers/" + offer.ID.String(), nil
	}
	return l.TargetURL, nil
}

// What is kept of it: a click, a bot's told apart.
func (LinkRules) Visited(ctx context.Context, m *Deals, l Link, v Visit) error {
	_, err := m.CreateClick(ctx, NewClick{LinkID: l.ID, Bot: v.Bot, Visitor: v.Visitor})
	return err
}
```

- `BeforeRedirect` answers where the visit goes: an address outside, or one
  of the site's (`/offers/1`, sent where the site is). `ErrGone` shows the
  site's page of a link that is no longer available (`410`), in its layout,
  with the words `goneTitle` and `goneText` of `messages.go`.
- `Visited` runs once the visit went on, or, with `wait`, once the page on
  the way was shown. An error is logged; the visit went on anyway.
- `Visit` has `Bot`, `Visitor`, `Referrer`, `UserAgent` and `At`. `Visitor`
  tells visits apart a day at a time without knowing who they are: an HMAC,
  with `SITE_VISITOR_SECRET`, of the address, the browser and the day, which
  keeps neither and changes every day.

`code` is not `required`, so that a link may be written without one and the
rule fills it; through the API too.

## The brand's links, and a short domain

The brand's fixed links (`/groups`, `/telegram`) are links with a code
written by hand: in the admin panel, or in a seed.

For a domain of its own, a short one, the redirects go on a site of their
own, with no pages:

```esquadro
blueprint ssr links {
  domain "pdd.ink"
}
```

and the entity says `@links(redirect, by code, to target_url)`.

## Counting

The clicks are records like any other: a report counts them by group and by
day, and the admin panel's dashboard draws it. Reading a record by any
unique field, as the redirect does, is `m.ReadLinkByCode(ctx, code)`.

## Tests

The site's generated tests visit a link's address, as a person (the page on
the way, with `wait`) and as a preview, and one of no link; they write the link themselves when it needs
no other record (one with a required relation is tested by hand, as the
example's, which needs an offer).
