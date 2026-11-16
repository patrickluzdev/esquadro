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
`confirm_email_change` and `email_changed` are security's, which you change
like any other.

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
