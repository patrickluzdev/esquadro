---
title: Security
description: Who signs in, what each one may do, and the defenses at the edge.
---

The [security](/esquadro/blueprints/reference/security/) blueprint keeps
security in one place. Without anyone who signs in, it answers CORS and sends
the security headers. Mark an entity `@security(principal)` and it also
generates sign-up, sign-in, sessions, tokens, password reset, social sign-in
and the checks of who may call each operation of the API. The `api` starter
has it, with a `User` and a `Customer`.

## Realms

Each entity marked `@security(principal)` is a **realm**: a kind of account,
with its own sign-up, sign-in, sessions and routes. A `User` and a `Customer`
never share an identity, even with the same email.

```esquadro
entity User {
  name string required
  @security(principal, roles [admin, staff], permissions [orders.refund],
            grants [admin.orders.refund], read user, write user.admin)
}

entity Customer {
  name string required
  @security(principal, signup open, providers [password, google])
}
```

| Setting | Values | Default |
|---|---|---|
| `signup` | `open` (anyone), `invite`, `closed` | `closed` |
| `providers` | `password`, `google`, `github` | `[password]` |
| `roles` | the realm's roles | none |
| `permissions` | named after what they are about and the action: `orders.refund` | none |
| `grants` | which role has which permission: `admin.orders.refund` | none |
| `verify_email` | `required` (no sign-in before), `optional`, `off` | `optional` |
| `password_reset` | `true`, `false` | `true` |
| `password_min`, `password_max`, `password_blocklist` | the realm's own password rules | the blueprint's |

An identity has the id of its principal: the `users` row of an account has the
id of its identity. The identities, credentials, sessions and the rest live in
`security_` tables (or the `security` schema, with the postgres blueprint's
`schemas ["security"]`), created by migrations of the blueprint.

## Who may do what

Every entity says who may `list`, `read`, `create`, `update` or `delete` it;
`write` is the last three. Each is `public` (anyone, signed in or not), a realm
(`user`: any of its accounts), a role (`user.admin`), or `owner`: the
principal the record belongs to.

```esquadro
entity Product {
  ...
  @security(read public, write [user.admin, user.staff])
}

entity Review {
  text     string required
  customer > Customer required
  @security(read [owner, user], create owner, update owner, delete [owner, user.admin])
}
```

- `list` follows `read`, and `create`, `update` and `delete` follow `write`.
- An entity without `@security` may be changed by whoever signed in, in any
  realm; `esquadro check` warns of it.
- An entity with rules, but none for an operation, lets no one do it.
- Linking and unlinking a many-to-many relation is an `update` of the record;
  listing it, a `read`.

### Owners

`owner` is the relation of the entity to a principal: `customer > Customer`
above. With more than one, `@security(owner buyer, ...)` says which; a
principal (`Customer` itself) owns its own record. With `owner`:

- a list holds only the records of whoever asks, unless another rule lets
  them see all (an admin sees every review); the query filters them, so pages
  stay whole;
- a record someone may not read answers `404`, as if it did not exist, and
  `401` to a request without a session or token: no one learns which ids
  exist;
- a record created by its owner is theirs, whatever the body says
  (`customer_id` is then optional in the API); someone let in by another rule,
  such as an admin, chooses the owner, creating on someone's behalf;
- an owner cannot hand a record over to someone else on update.

### Policies

Each entity has a policy, in `internal/<module>/<entity>_policy.go`, that the
API asks before each operation: `List` (a scope: all, the owner's, or none),
`Read`, `Create`, `Update` and `Delete`. Its rules come from the model, in
`<entity>_policy_gen.go`, generated again with it; the file without `_gen` is
yours. Write an action there and it replaces the model's:

```go
// Only open orders change, and only by their customer.
func (p OrderPolicy) Update(ctx context.Context, o Order) bool {
	return p.orderRules.Update(ctx, o) && o.Status == StatusOpen
}
```

Ask the policy in your own code too: `sales.OrderPolicy{}.Update(ctx, order)`.
The generated tests check the model's rules; `TestAPIAccess` checks that the
API answers as the policy that runs says, yours included.

Each operation of the API answers `401 unauthorized` to a request without a
session or token that may not, and `403 forbidden` to one with a session;
its description in `openapi.yaml` says who may. In your own code,
`security.Require(ctx, "orders.refund")` answers the same problems, by the
roles' permissions; `security.Allows(ctx, "user.admin")` checks a rule. An
operation of your own, which is not an entity's, needs someone signed in.

## Signing in

The routes of each realm are under `/identity/<realm>/` (the `path` option),
described by `/identity/openapi.yaml`; with `api_docs`, `/identity/docs` shows
them as `/docs` shows the API. Every JSON name is snake_case.

| Route | |
|---|---|
| `POST sign-up` | `{"email", "password", ...fields of the entity}`; answers `202` whether the email had an account or not |
| `POST sign-in` | `{"email", "password"}`: a session cookie and its `csrf_token`; with `"cookie": false`, a `session_token` for an `Authorization: Bearer` header |
| `POST sign-out` | ends the session |
| `GET session` | who signed in: `id`, `realm`, `roles`, `permissions`, `csrf_token` |
| `POST token` | a JWT of 15 minutes (`token_minutes`), for other services; they check it with `/.well-known/jwks.json` |
| `GET sessions`, `DELETE sessions`, `DELETE sessions/{id}` | the account's devices; ending the others needs a recent sign-in |
| `POST password` | `{"current_password", "new_password"}`; ends the other sessions |
| `POST verify-email/send`, `POST verify-email` | the verification link, and its `{"token"}` |
| `POST password/forgot`, `POST password/reset` | the reset link, and `{"token", "new_password"}` |
| `POST email`, `POST email/confirm` | a new email, confirmed by a link sent to it |
| `GET oauth/<provider>?redirect=` | sign in with Google or GitHub |
| `GET providers`, `POST providers/<provider>`, `DELETE providers/<provider>` | the ways the account signs in |
| `DELETE` (the realm's path) | deletes the account, after a recent sign-in |

**Browsers** use the session cookie (`HttpOnly`, `SameSite=Lax`, named
`__Host-<app>_<realm>_session` outside development). A request that changes
something must come from the app's origin (or one of `origins`) and send the
session's `csrf_token` in `X-CSRF-Token`. **Other clients** sign in with
`"cookie": false` and send `Authorization: Bearer <session_token>`, or a JWT.

Sessions end after 24 hours without use (`session_idle_hours`) and 30 days in
all (`session_max_days`). Failed sign-ins lock an account for 15 minutes after
5 (`lockout_attempts`, `lockout_minutes`), except in development.

## Passwords

Passwords follow NIST SP 800-63B and the OWASP Password Storage Cheat Sheet:

- **Argon2id** (19 MiB, 2 iterations), stored in the PHC format and hashed
  again when the costs go up; an optional pepper (`pepper true`,
  `SECURITY_PEPPER`), kept out of the database.
- **At least 12 characters** (`password_min`), up to 128; any character,
  spaces and emoji included; no rules of composition.
- **Blocklists** (`password_blocklist`): `none` by default; `lists` refuses the
  passwords of your files (`password_lists ["config/passwords-br.txt"]`, for
  the language and country of your users), sequences, repetitions, the app's
  name and the person's email; `pwned` also asks Pwned Passwords, which only
  sees the first five characters of the password's SHA-1.

## Google and GitHub

Add `google` or `github` to a realm's `providers`, and register the app at the
provider with the redirect URI `<public_url>/identity/<realm>/oauth/<provider>/callback`.
Its client id and secret go in `SECURITY_GOOGLE_CLIENT_ID` and
`SECURITY_GOOGLE_CLIENT_SECRET` (or `SECURITY_GITHUB_...`), which exist only
when a realm uses the provider.

The app sends the browser to `GET /identity/<realm>/oauth/google?redirect=/home`
and gets it back at `/home`, signed in; or at `/home?error=<code>`. The flow
uses PKCE and a state bound to the browser. An account is linked to an
existing one by email only when both sides verified it; otherwise the person
signs in and links the provider. `redirect` is a path of the app or an address
of `redirect_urls`.

## Emails

Verification, password reset and email change send links to a page of your
app (`redirect`), which posts the token back. They go by the
[mail](/esquadro/email/) blueprint, which an app with realms needs: from the
templates `verify_email`, `reset_password`, `confirm_email_change` and
`email_changed`, which are yours to change; in development, to Mailpit.

With the [jobs](/esquadro/blueprints/reference/jobs/) blueprint, an hourly job
deletes expired sessions and links, and rotates the JWT signing key every
`rotate_keys_days`. Without jobs, the
[worker](/esquadro/blueprints/reference/worker/) does it; without either, run
`<app> security cleanup` on a schedule.

## Commands

```sh
go run ./cmd/shop security create user ana@example.com name=Ana -role admin
go run ./cmd/shop security grant user ana@example.com staff
go run ./cmd/shop security revoke user ana@example.com admin
go run ./cmd/shop security sessions user ana@example.com
go run ./cmd/shop security delete customer bia@example.com
go run ./cmd/shop security cleanup
go run ./cmd/shop security rotate-keys
```

`create` prints a link that sets the password; with `-password-stdin` it reads
the password instead, never from the arguments.

## Tests

- The generated API tests run as everyone: an account of each realm with every
  role. `TestAPIAccess` runs every operation as no one, each realm and each
  role, against the rules of the model.
- A request with `X-Test-Principal: user.staff` (or `nobody`) is signed in as
  that one alone, in the API tests.
- `securitytest.As(ctx, "user", "admin")` signs a context in, for code that
  calls `security.Require`; `securitytest.SignIn(t, s, "user", "admin")` makes
  an account and a session, and answers its `Authorization` header.

## At the edge

- **CORS**: the `origins` option (`<APP>_CORS_ORIGINS`). A listed origin may
  send cookies; `"*"` lets any in, without them.
- **Headers**: `nosniff`, `DENY` framing, a `default-src 'none'` CSP, no
  referrer, `no-store`, and HSTS with `hsts true` (`<APP>_HSTS`), behind HTTPS
  only.
