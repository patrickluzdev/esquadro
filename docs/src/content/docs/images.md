---
title: Drawn images
description: "Images the app draws on the server: each record's Open Graph image, 1200x630, drawn in the brand's colors and fonts and kept in the store, and a kit to draw your own."
---

A link to a record's page, shared in a chat or a social network, shows a
picture: the page's `og:image`. A product's photo is a poor one (it is not
1200x630, and it says nothing of the product's name); a picture made by
hand for each record does not scale. The `images` blueprint has the app
**draw it**, from the record, in the site's brand:

```esquadro
app shop {
  blueprint storage
  blueprint ui
  blueprint ssr site {
    brand "brand"
  }
  blueprint images
}
```

```esquadro
entity Product {
  name        string required
  description text
  photo       image
  @site(page, description description, og_image)
}
```

`og_image` on a site's page gives each record's page an image of its own,
at `/<path>/<id>/og.png` (`/products/<id>/og.png`): a PNG of 1200x630, the
size the sites that show shared links ask for. The record's page points
its `og:image` at it (with its width and height), in place of the
record's picture.

## How it is drawn

`web/<site>/product_og.go` draws it, and it is **yours**: esquadro writes
it once and merges its changes into yours.

```go
const productOGVersion = "1"

func DrawProductOG(c *images.Canvas, o ProductOG) error {
	w, h := c.Bounds().Dx(), c.Bounds().Dy()
	c.Fill(c.Color("page"))
	c.Rect(image.Rect(0, h-12, w, h), c.Color("accent"), 0)
	// ...
	c.Text(image.Rect(72, 172, 620, 546), o.Title,
		images.Text{Font: "display", Size: 76, MinSize: 44, Line: 1.1, MaxLines: 3, Color: c.Color("ink")})
	return nil
}
```

What it draws by default: the brand's page color; the brand's logo, or
the site's name; the record's title in the display font, as large as it
fits in three lines; its description, when the page has one, in what is
left; its picture (the page's `image` field), whole, on a card on the
right; and a band of the accent at the bottom. `ProductOG` has the record
(`o.Item`), its title and description as the page has them, its picture
(`o.Picture`, nil without one), the site's name and the logo.

## Kept in the store

Drawing an image takes milliseconds, but a link is shared many times. The
first request draws it and keeps it in the [storage](/esquadro/files/) store
(`images/<site>/products/<id>/<name>.png`); the next ones are served from
there. Its name is made of what it is drawn from:

- the record's `updated_at`: a change to the record draws it again;
- `productOGVersion`, in `product_og.go`: change it when you change the
  drawing, and every image is drawn again;
- the theme's: the colors, and the content of the brand's font and logo
  files. A change to the brand draws them again by itself.

The versions before are deleted when a new one is kept. The image's
address carries its name too (`og.png?v=...`), so that a shared link's
preview, kept by the network, changes with the record. Browsers and
proxies keep the image for an hour (shared caches only when everyone may
read the record), and one asked for again with its `ETag` is answered
`304`.

An image that cannot be drawn (the record's picture cannot be read, the
drawing failed) is logged, and the request is sent to the brand's
`identity.og_image`, or answered `500` without one.

## Fonts and colors

The images take the site's brand: its colors by role (`c.Color("page")`,
`"surface"`, `"ink"`, `"muted"`, `"accent"`, `"accent_text"`, `"line"` and
the brand's own) and its fonts, `"display"` and `"body"`. Without a brand,
they take the neutral look, with the site's `primary` as the accent.

The images are drawn in pure Go (`golang.org/x/image`, no C library to
install), which reads **TrueType and OpenType** files (`.ttf`, `.otf`):

- of the brand's files of a font, the one of the weight nearest a title's
  (700) or a text's (400) is drawn;
- a variable font (`"100 900": "fonts/Inter.ttf"`) is drawn in its default
  instance, often its regular weight: give the font a static bold file to
  draw titles bold;
- WOFF and WOFF2 files are not read: `esquadro check` warns, and the
  images draw the Go fonts in their place.

The logo is drawn when it is a PNG, JPEG, GIF or WebP (the symbol, when
only it is); an SVG logo is not, and the site's name is written instead.

## Your own images

`internal/platform/images` is a small kit any part of the app can draw
with: a square image of a post, a coupon, a badge.

```go
c := images.New(1080, 1080, theme)
c.Fill(c.Color("page"))
c.Rect(image.Rect(60, 60, 1020, 1020), c.Color("surface"), 32)
c.Picture(image.Rect(100, 100, 980, 640), photo, images.Cover, 24)
c.Text(image.Rect(100, 680, 980, 980), title,
	images.Text{Font: "display", Size: 72, MinSize: 40, MaxLines: 3, Align: images.Center})
png, err := c.PNG()
```

| | |
|---|---|
| `images.New(w, h, theme)` | a canvas; `theme` has the colors and fonts (`images.LoadFonts(fsys, files)`), the Go fonts where it has none |
| `c.Fill(color)`, `c.Rect(r, color, radius)` | paint the canvas, a rectangle with round corners |
| `c.Text(r, s, images.Text{...})` | text in its box: wrapped at spaces (and inside a word longer than a line), shrunk down to `MinSize`, cut with an ellipsis after `MaxLines` or the box's height; it answers the rectangle it took |
| `c.Layout(s, width, height, t)`, `images.Wrap(face, s, width, lines)` | the lines without drawing them |
| `c.Picture(r, img, images.Contain or images.Cover, radius)` | a picture in a box, whole or filling it |
| `images.Decode(data)`, `images.Fetch(ctx, url)` | a PNG, JPEG, GIF or WebP from bytes, or from an address |
| `images.Hex("#2a85ff")` | a color |
| `c.PNG()` | the image |
| `images.Cache{Store: store}.PNG(ctx, folder, name, draw)` | the image kept in the store under `folder/name`, drawn and kept when there is none |

A site's theme is `s.og`, in the site's package.
