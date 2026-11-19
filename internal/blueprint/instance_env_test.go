package blueprint

import (
	"testing"

	"github.com/patrickluzdev/esquadro/internal/project"
)

func TestInstanceEnvNamesEachInstance(t *testing.T) {
	env := []*project.EnvVar{
		{Name: "INSTANCE_URL", Blueprint: "integration", Field: "InstanceURL"},
		{Name: "SHARED", Blueprint: "integration"},
	}
	got := instanceEnv(env, "mercado-pago")
	if got[0].Name != "MERCADO_PAGO_URL" || got[0].Field != "" || got[0].Blueprint != "mercado-pago" {
		t.Errorf("INSTANCE_URL: %+v", got[0])
	}
	if got[1].Name != "SHARED" || got[1].Blueprint != "mercado-pago" {
		t.Errorf("SHARED: %+v", got[1])
	}
	if env[0].Name != "INSTANCE_URL" {
		t.Error("the manifest's variables changed")
	}
}
