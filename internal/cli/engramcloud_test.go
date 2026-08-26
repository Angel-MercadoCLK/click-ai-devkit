package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/Angel-MercadoCLK/click-ai-devkit/internal/installer"
)

func TestResolveCloudTokenPersistence_YesFlagNeverPersistsToken(t *testing.T) {
	tests := []struct {
		name        string
		yesFlag     bool
		persistFlag bool
		wantMode    installer.CloudTokenPersistence
	}{
		{
			name:        "yes flag without dedicated opt-in yields decline mode",
			yesFlag:     true,
			persistFlag: false,
			wantMode:    installer.CloudTokenPersistenceDecline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := resolveCloudTokenPersistence(tt.yesFlag, tt.persistFlag, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Errorf("resolveCloudTokenPersistence() mode = %v, want %v", mode, tt.wantMode)
			}
		})
	}
}

func TestResolveCloudTokenPersistence_DedicatedFlagPersistsToken(t *testing.T) {
	tests := []struct {
		name        string
		yesFlag     bool
		persistFlag bool
		wantMode    installer.CloudTokenPersistence
	}{
		{
			name:        "yes flag with dedicated opt-in yields persist mode",
			yesFlag:     true,
			persistFlag: true,
			wantMode:    installer.CloudTokenPersistencePersist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, err := resolveCloudTokenPersistence(tt.yesFlag, tt.persistFlag, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Errorf("resolveCloudTokenPersistence() mode = %v, want %v", mode, tt.wantMode)
			}
		})
	}
}

func TestResolveCloudTokenPersistence_PromptStatesDestinationAndPermissions(t *testing.T) {
	var buf bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("y\n"))
	restore := SetReadCloudTokenConsentFuncForTests(func(r *bufio.Reader) (bool, error) {
		buf.WriteString(consentPrompt)
		return true, nil
	})
	defer restore()

	mode, err := resolveCloudTokenPersistence(false, false, reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != installer.CloudTokenPersistencePersist {
		t.Errorf("resolveCloudTokenPersistence() mode = %v, want %v", mode, installer.CloudTokenPersistencePersist)
	}
	prompt := buf.String()
	if !strings.Contains(prompt, "~/.claude/settings.json") {
		t.Error("prompt does not contain ~/.claude/settings.json")
	}
	if !strings.Contains(prompt, "0600") {
		t.Error("prompt does not contain 0600")
	}
}

func TestResolveCloudTokenPersistence_NonAffirmativeOmitsToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMode installer.CloudTokenPersistence
	}{
		{
			name:     "n answer declines",
			input:    "n\n",
			wantMode: installer.CloudTokenPersistenceDecline,
		},
		{
			name:     "N answer declines",
			input:    "N\n",
			wantMode: installer.CloudTokenPersistenceDecline,
		},
		{
			name:     "empty answer declines",
			input:    "\n",
			wantMode: installer.CloudTokenPersistenceDecline,
		},
		{
			name:     "random answer declines",
			input:    "maybe\n",
			wantMode: installer.CloudTokenPersistenceDecline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			mode, err := resolveCloudTokenPersistence(false, false, reader)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Errorf("resolveCloudTokenPersistence() mode = %v, want %v", mode, tt.wantMode)
			}
		})
	}
}

func TestResolveCloudTokenPersistence_ReaderErrorSkipsTokenAndWarns(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("y\n"))
	restore := SetReadCloudTokenConsentFuncForTests(func(r *bufio.Reader) (bool, error) {
		return false, fmt.Errorf("simulated reader error")
	})
	defer restore()

	mode, err := resolveCloudTokenPersistence(false, false, reader)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if mode != installer.CloudTokenPersistenceDecline {
		t.Errorf("resolveCloudTokenPersistence() mode = %v, want %v", mode, installer.CloudTokenPersistenceDecline)
	}
}
