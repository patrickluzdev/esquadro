package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An app with observability measures its queue for the platform's alerts:
// the jobs by state, those given up, and those on a schedule that are late,
// with the table that keeps their last runs. An integration with check
// true is checked by the worker, and each process tells itself apart.
func TestAppMeasuresWhatTheAlertsWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a project with the store blueprint, which needs sqlc or Docker")
	}
	dir := filepath.Join(t.TempDir(), "promo")
	if code, _, errOut := esq(t, "new", "-starter", "api", "-module", "example.com/promo", "-no-git", "-no-gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	src := read(t, appEsq)
	at := strings.LastIndex(src, "}")
	src = src[:at] + "  blueprint integration telegram {\n    url   \"https://api.telegram.org\"\n    key   true\n    check true\n  }\n  blueprint integration shopee {\n    url \"https://x.com\"\n  }\n" + src[at:]
	if err := os.WriteFile(appEsq, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	health := read(t, filepath.Join(dir, "internal", "platform", "jobs", "health.go"))
	for _, want := range []string{`"jobs.queue"`, `"jobs.periodic.overdue"`, `"jobs.discarded"`, "jobs_periodic_runs"} {
		if !strings.Contains(health, want) {
			t.Errorf("health.go lacks %s", want)
		}
	}
	if !strings.Contains(read(t, filepath.Join(dir, "internal", "platform", "jobs", "jobs.go")), "health.watch()") {
		t.Error("the queue does not watch its health")
	}
	if !strings.Contains(read(t, filepath.Join(dir, "migrations", "migrations.go")), "CREATE TABLE IF NOT EXISTS jobs_periodic_runs") {
		t.Error("the migrations do not make jobs_periodic_runs")
	}
	if !strings.Contains(read(t, filepath.Join(dir, "internal", "platform", "telemetry", "telemetry.go")), `"service.instance.id"`) {
		t.Error("the processes do not tell themselves apart")
	}
	worker := read(t, filepath.Join(dir, "cmd", "promo", "worker.go"))
	if !strings.Contains(worker, `{Name: "telegram check", Run: d.telegram.Watch(d.logger)}`) || strings.Contains(worker, "shopee check") {
		t.Errorf("the worker checks the integrations with check true, and only those:\n%s", worker)
	}
	if watch := read(t, filepath.Join(dir, "internal", "integrations", "telegram", "watch.go")); !strings.Contains(watch, `"integration.check"`) {
		t.Errorf("watch.go records no integration.check:\n%s", watch)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "integrations", "shopee", "watch.go")); !os.IsNotExist(err) {
		t.Errorf("watch.go without check: %v", err)
	}
	if client := read(t, filepath.Join(dir, "internal", "integrations", "telegram", "client.go")); !strings.Contains(client, "strings.TrimSpace(cfg.Secrets.Key)") {
		t.Error("the client keeps the spaces around its key")
	}
}

// The platform's Grafana gets each app's dashboard and alerts, in the
// language and with the limits of the blueprint's options, and a Telegram
// contact point whose token stays in Grafana's environment.
func TestPlatformAlertsTheApps(t *testing.T) {
	dir := platformProject(t)
	model := filepath.Join(dir, "esquadro", "platform.esq")
	src := strings.Replace(read(t, model), "  blueprint observability\n", "  blueprint observability {\n    alerts_language \"pt-BR\"\n    alerts_restarts 5\n  }\n", 1)
	if err := os.WriteFile(model, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatalf("gen: %s%s", out, errOut)
	}
	vars := read(t, filepath.Join(dir, "ansible", "group_vars", "all", "platform.yml"))
	for _, want := range []string{`observability_alerts_language: "pt-BR"`, "observability_alerts_restarts: 5", "observability_alerts_backup_hours: 26"} {
		if !strings.Contains(vars, want) {
			t.Errorf("platform.yml lacks %s:\n%s", want, vars)
		}
	}
	if secrets := read(t, filepath.Join(dir, "secrets", "platform.example.yaml")); !strings.Contains(secrets, "alerts_telegram_token:") || !strings.Contains(secrets, "alerts_telegram_chat_id:") {
		t.Errorf("the secrets lack the Telegram bot:\n%s", secrets)
	}
	role := filepath.Join(dir, "ansible", "roles", "observability")
	alerts := read(t, filepath.Join(role, "templates", "alerts.yaml.j2"))
	for _, want := range []string{"type: telegram", "${ALERTS_TELEGRAM_TOKEN}", "river_work_count_total", "jobs_periodic_overdue", "integration_check", "platform_backup_last_success"} {
		if !strings.Contains(alerts, want) {
			t.Errorf("alerts.yaml.j2 lacks %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(role, "templates", "dashboard.json.j2")); err != nil {
		t.Error(err)
	}
	tasks := read(t, filepath.Join(role, "tasks", "main.yml"))
	if !strings.Contains(tasks, "ALERTS_TELEGRAM_TOKEN={{ alerts_telegram_token }}") || !strings.Contains(tasks, "src: dashboard.json.j2") {
		t.Errorf("the tasks give Grafana no token or no dashboards:\n%s", tasks)
	}
	if compose := read(t, filepath.Join(role, "templates", "compose.yaml.j2")); !strings.Contains(compose, `"127.0.0.1:4318:4318"`) {
		t.Error("OTLP is not open to the server itself, for the backup's report")
	}
}
