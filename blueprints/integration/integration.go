// Package integration holds the Go side of the integration blueprint: what
// its templates know of an instance's options, and their checks.
package integration

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the functions the integration blueprint's templates
// call.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"integrationField": Field,
		"integrationEnv":   EnvName,
		"pascalOf":         Pascal,
		"rateOf": func(s string) string {
			n, per, _ := Rate(s)
			if n == 0 {
				return "0, 0"
			}
			return fmt.Sprintf("%d, %s", n, goDuration(per))
		},
		"durationOf": func(s string) string {
			d, _ := time.ParseDuration(s)
			return goDuration(d)
		},
	}
}

// EnvName is the name of one of an instance's variables: SHOPEE_URL.
func EnvName(instance, suffix string) string {
	return strings.ToUpper(strings.ReplaceAll(instance, "-", "_")) + "_" + suffix
}

// Field is the field of config.Config that holds one of an instance's
// variables: ShopeeURL.
func Field(instance, suffix string) string {
	return project.FieldName(EnvName(instance, suffix))
}

// Pascal is an instance's name as Go exports it: Shopee.
func Pascal(instance string) string {
	return project.FieldName(strings.ToUpper(instance))
}

var rate = regexp.MustCompile(`^(\d+)/(s|min|h)$`)

// Rate reads a rate limit as the option writes it: 2/s, 120/min, 1000/h;
// "0" is none.
func Rate(s string) (int, time.Duration, error) {
	if s == "0" || s == "" {
		return 0, 0, nil
	}
	m := rate.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("rate_limit %q is not N/s, N/min or N/h", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, 0, fmt.Errorf("rate_limit %q is not N/s, N/min or N/h", s)
	}
	per := map[string]time.Duration{"s": time.Second, "min": time.Minute, "h": time.Hour}[m[2]]
	return n, per, nil
}

func goDuration(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%d * time.Hour", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%d * time.Minute", d/time.Minute)
	case d%time.Second == 0:
		return fmt.Sprintf("%d * time.Second", d/time.Second)
	}
	return fmt.Sprintf("%d * time.Millisecond", d/time.Millisecond)
}

var name = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// Check checks each integration: a name that is a Go package, an http(s)
// url, its rate limit, timeout and retries.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	for _, b := range set.Blueprints {
		if b.Kind != "integration" {
			continue
		}
		at := func(key string) diag.Pos {
			for _, o := range b.Ref.Options {
				if o.Key.Name == key {
					return o.Pos
				}
			}
			return b.Ref.Pos
		}
		inst, opts := b.Manifest.Name, set.Options[b.Manifest.Name]
		if !name.MatchString(inst) {
			diags.Errorf(b.Ref.Pos, b.Ref.Pos, "%s cannot name an integration: lowercase letters and digits, as a Go package (shopee, mercadopago)", inst)
		}
		if address, _ := opts["url"].(string); address == "" {
			diags.Errorf(b.Ref.Pos, b.Ref.Pos, "integration %s needs its url, the API's address: blueprint integration %s { url \"https://...\" }", inst, inst)
		} else if u, err := url.Parse(address); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			diags.Errorf(at("url"), at("url"), "url of %s: %q is not an http(s) address", inst, address)
		}
		if s, _ := opts["rate_limit"].(string); s != "" {
			if _, _, err := Rate(s); err != nil {
				diags.Errorf(at("rate_limit"), at("rate_limit"), "%s: %v", inst, err)
			}
		}
		if s, _ := opts["timeout"].(string); s != "" {
			if d, err := time.ParseDuration(s); err != nil || d <= 0 {
				diags.Errorf(at("timeout"), at("timeout"), "timeout of %s: %q is not a duration (30s, 1m)", inst, s)
			}
		}
		if n, _ := opts["retries"].(int64); n < 0 || n > 10 {
			diags.Errorf(at("retries"), at("retries"), "retries of %s: %d is not between 0 and 10", inst, n)
		}
		for _, m := range p.Modules {
			if m.Snake == inst {
				diags.Errorf(b.Ref.Pos, b.Ref.Pos, "integration %s has the name of a module: give one of them another", inst)
			}
		}
	}
	return diags
}
