---
title: Email
description: Send emails from templates, by SMTP or Resend, with Mailpit in development.
---

The [mail](/esquadro/blueprints/reference/mail/) blueprint sends a project's
emails: those of your modules ("order confirmed", "invitation") and those of
[security](/esquadro/security/) (verification, password reset, email
change). The `api` starter has it; `esquadro add mail` adds it to any
project.

## Where emails go

`MAIL_URL` says how emails go, in each environment, without generating
again:

| `MAIL_URL` | |
|---|---|
| `smtp://user:password@host:port` | any SMTP server (Amazon SES, Resend, Postmark, your own): TLS from the start on port 465, STARTTLS on the others; without TLS only in development |
| `resend://re_...` | Resend's API, with an idempotency key: a retried email is never sent twice |
| `log://` | into the log |

`MAIL_FROM` is who emails come from (`Shop <no-reply@shop.com>`). Both are
required outside development, and the app refuses to start when one is
wrong. In development, with [compose](/esquadro/blueprints/reference/compose/),
emails go to **Mailpit**, which `task up` starts: read them at
http://localhost:8025. Without compose, they go into the log.

Check a provider with `go run ./cmd/<app> mail test you@example.com`: it sends
an email now, with the environment's configuration.

## Templates

Each email is a template of the project, in
`internal/platform/mail/templates/`, written once and then yours:

- `<name>.txt`: the first line is the subject, the rest the text.

  ```
  Subject: Your order {{.Number}} is confirmed
  Hi {{.Name}},

  We got your order {{.Number}}.
  ```

- `<name>.html` (optional): the HTML version, inside `_layout.html`.

  ```html
  {{define "content"}}
  <p>Hi {{.Name}},</p>
  <p>We got your order {{.Number}}.</p>
  {{end}}
  ```

- `<name>.json` (optional): data to try it with. The generated
  `TestTemplates` writes every template with its data, and fails when one
  names a field the data does not have: a mistake shows in `go test`, not in
  a customer's inbox.

`welcome` is an example; `verify_email`, `reset_password`,
`confirm_email_change`, `email_changed` and `invitation` are security's,
which you change like any other.

## The layout

Every HTML email is written inside `_layout.html`, and every text inside
`_layout.txt` when it is there; both are in the same folder, written once
and then yours. They write the look with the functions of the generated
`layout.go`, which every template may use too:

| | |
|---|---|
| `branded` | whether mail has a brand |
| `color "accent"` | a color by role: `page`, `surface`, `ink`, `muted`, `line`, `accent`, `on_accent` |
| `font "body"` | a font family: `display` or `body` |
| `logo` | the logo's address (`cid:brand-logo`), empty without one |
| `app`, `address` | the app's name, and the footer's last line |
| `word "why"` | a word of the layout, in mail's `locale` |
| `button .Link "Choose a password"` | a link as a button, a paragraph of its own; in a text, the label and the link |

Without a brand, emails keep their neutral look: a white card on a light
gray page, in the system's fonts, with no header nor footer but the
`address`, when mail has one.

### A layout with the brand

`brand` names the project's folder of its brand, the same one an
[ssr site](/esquadro/frontends/#a-site-with-a-brand) may name:

```esquadro
blueprint mail {
  brand "brand"
  locale pt_br
  address "Shop Ltd, 1 Main Street, Springfield"
}
```

The layout then takes from its `theme.json`:

- **colors**: the page, the card (`surface`, edged by `line`), the text
  (`ink`, the footer `muted`) and the buttons (`accent`, `on_accent`); a
  role left out keeps the default look's.
- **fonts**: the `display` font (the header) and the `body` font, by their
  family's name, then their `fallback` or the system's fonts. Emails load
  no fonts: a reader's device with the font shows it, any other the next.
- **the logo**, in the header: `identity.logo_email`, or `identity.logo`
  when it is a PNG, JPEG or GIF. Email clients do not show SVG, and many
  block pictures from the web, so the logo goes inside each email that
  shows it (`cid:brand-logo`, by SMTP and Resend alike), from the brand's
  folder embedded in the app (`brand/embed.go`). Without one the header
  writes the app's name, and `esquadro check` warns of a logo that is only
  an SVG.

Under the card, the footer writes the app's name, why the reader got the
email ("You are receiving this email from Shop about your account", in
`locale`: `en`, `pt_br`, or the first frontend's by default) and
`address`, when there is one (some laws ask for a postal address). The
text gets the same footer, in plain text. A template changes the reason
with a `why` of its own:

```html
{{define "why"}}You are receiving this email because you signed up for our news.{{end}}
```

The layout is tables with the styles inline, at most 560 px wide: what
every email client reads.

### Your own layout

`_layout.html` and `_layout.txt` are yours: change them, or write them
anew, and esquadro keeps them. While yours is still as esquadro wrote it,
`esquadro gen` brings it the template's changes; once you change it, they
are merged into it. To leave the brand out, write a layout without its
functions (or with colors of its own); to leave the text's footer out,
delete `_layout.txt`, which esquadro does not bring back.

## Sending

Each module gets the mailer in its dependencies, `Mail`:

```go
err := m.Mail.Send(ctx, mail.Email{
	To:       []string{customer.Email},
	Template: "order_confirmed",
	Data:     map[string]any{"Name": customer.Name, "Number": order.Number},
})
```

`Send` writes the email now, so that a template that fails, fails there, and
sends it in the background:

- **With [jobs](/esquadro/blueprints/reference/jobs/)**, each email is a job,
  retried until it goes. It is sealed with `MAIL_SECRET`: nothing of it, an
  address or a link, shows in the queue's table. `SendTx(ctx, tx, email)`
  sends it only if the transaction commits: no "order confirmed" for an order
  that was not stored.
- **Without jobs**, it goes in the background of the process.

## Tests

`mailtest.Capture(t)` is a mailer that keeps what it would send:

```go
mailer, sent := mailtest.Capture(t)
// ... code that sends an email ...
email := sent.Last("ana@example.com")
link, query := mailtest.Link(t, email)
```
