package initcmd

import "testing"

func TestKitHome_EnvOverride(t *testing.T) {
	t.Setenv("UNITY_CLI_AGENTKIT_HOME", "/opt/kit")
	got, err := KitHome()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/opt/kit" {
		t.Fatalf("KitHome = %q, want /opt/kit", got)
	}
}
