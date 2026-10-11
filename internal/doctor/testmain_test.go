package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "click-doctor-test-home-*")
	if err != nil {
		panic(err)
	}
	for key, value := range map[string]string{"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, ".config")} {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	}
	p := installer.NewGentleAIProvisioner()
	p.HomeDir = func() (string, error) { return home, nil }
	p.Lstat = os.Lstat
	p.Run = func(context.Context, []string, string, ...string) ([]byte, error) {
		panic("doctor tests must inject the managed Gentle AI probe")
	}
	managedGentleAIProvisionerFactory = func() installer.GentleAIProvisioner { return p }
	code := m.Run()
	_ = os.RemoveAll(filepath.Clean(home))
	os.Exit(code)
}
