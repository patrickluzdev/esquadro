---
title: Integrations
description: "Clients of outside APIs, one per API: retries, a rate limit, a timeout, secrets from the environment, tracing, and a fake API for tests."
---

An app talks to APIs it does not own: an affiliate program's offers, a
messaging bot, a payment provider. Each needs the same care: trying again
when the API fails for a moment, staying under its limit, giving up in
time, keeping its secrets out of logs, and being testable without calling
it. The `integration` blueprint gives each API a client with all of that,
by name:

```esquadro
app promo {
  blueprint integration shopee {
    url        "https://open-api.affiliate.shopee.com.br"
    rate_limit "2/s"
    timeout    "20s"
    key        true
    secret     true
  }
  blueprint integration telegram {
    url        "https://api.telegram.org"
    rate_limit "25/s"
    key        true
  }
}
```

| Option | | Default |
|---|---|---|
| `url` | the API's address | required |
| `rate_limit` | at most `N/s`, `N/min` or `N/h` requests, in this process; `"0"`, none | `"0"` |
| `retries` | how many more times a call is tried when the API fails for a moment | `3` |
| `timeout` | how long a call may take, its tries included | `"30s"` |
| `key`, `secret` | the API takes a key (an app id, a bot's token) and a secret | `false` |

## Its variables

Each integration has its own: `SHOPEE_URL` (the `url` by default; point it
elsewhere per environment), and, as the options say, `SHOPEE_KEY` and
`SHOPEE_SECRET`, secrets required outside development. `esquadro env list`
shows them, and `go run ./cmd/promo shopee check` calls the API with an
environment's values, to see that they work.

## What is generated, and what is yours

`internal/integrations/shopee/client.go` is generated: a `Client` with
`JSON(ctx, method, path, in, out)` and `Do(req, body)`.

- A call that fails for a moment (429, 502, 503, 504, a connection lost) is
  tried again, waiting longer each time (200ms, 400ms, 800ms…, with a part
  at random), or as long as `Retry-After` says.
- GET, HEAD, PUT and DELETE are tried again. A POST is tried again only when
  the API says it did nothing (429, 503), or when the call is marked
  `shopee.Retry`: a GraphQL query, a POST with an idempotency key.
- The rate limit makes a call wait its turn; the timeout ends it, tries and
  all.
- An answer that is not a success is `*shopee.Error{Status, Body}`, and no
  error shows the key or the secret.
- With the `observability` blueprint, each call is a span
  (`shopee POST /graphql`) with the HTTP client's metrics.

`internal/integrations/shopee/shopee.go` is yours, written once:

```go
// Authenticate adds what the API needs to each request, on every try.
func Authenticate(req *http.Request, s Secrets) error {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	body, _ := io.ReadAll(req.Body) // the body is sent again after reading
	req.Body = io.NopCloser(bytes.NewReader(body))
	sum := sha256.Sum256([]byte(s.Key + ts + string(body) + s.Secret))
	req.Header.Set("Authorization", "SHA256 Credential="+s.Key+", Timestamp="+ts+", Signature="+hex.EncodeToString(sum[:]))
	return nil
}

// Offers are the API's offers of keyword.
func (c *Client) Offers(ctx context.Context, keyword string) ([]Offer, error) {
	var answer struct{ Data struct{ ProductOfferV2 struct{ Nodes []Offer } } }
	err := c.JSON(ctx, http.MethodPost, "/graphql", query(keyword), &answer, Retry)
	return answer.Data.ProductOfferV2.Nodes, err
}
```

A bot's token in the path is the same: `req.URL.Path = "/bot" + s.Key +
req.URL.Path` in `Authenticate`. `Check` is the call `shopee check` makes.

## Using it

Every module gets each integration in its dependencies: in a job, a
business rule or a route of yours, `m.Shopee.Offers(ctx, "panela")`.

A job that calls an API leaves to the client the failures of a moment, and
to the job's own retries the longer ones: an API down for an hour is tried
again by the queue, not by the call.

## Testing

`shopeetest.New(t, handler)` serves a fake Shopee API for the test, and
answers a client of it that does not wait between tries, with the requests
it got:

```go
c, api := shopeetest.New(t, shopeetest.JSON(200, `{"data":{"productOfferV2":{"nodes":[]}}}`))
m := deals.New(deals.Deps{Pool: pool, Shopee: c})
// ... run the job ...
if len(api.Requests()) != 1 { t.Fatal("no call to Shopee") }
```

The generated tests check the client itself: a GET tried again after a 503,
a POST after a 429 but not after a 502 unless marked `Retry`, giving up
after `retries`, the timeout, the rate limit, and that an error never shows
a secret.

The rate limit counts in each process: with more than one worker, each has
its own.
