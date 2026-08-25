package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"kekkai/vault"
)

// TestUserImportFile imports the manual-testing file test-import.csv
// (gitignored) through the real settings flow. It skips when the file is
// absent so CI stays green; create the file locally to run this check.
func TestUserImportFile(t *testing.T) {
	data, err := os.ReadFile("test-import.csv")
	if err != nil {
		t.Skip("manual test file not present")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "test-import.csv"), data, 0600); err != nil {
		t.Fatal(err)
	}
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prevWd)

	path := filepath.Join(tmp, "vault.kek")
	master := []byte("master-pass")
	if err := vault.New(path, master); err != nil {
		t.Fatal(err)
	}

	m := &tuiModel{path: path, master: append([]byte(nil), master...), entries: nil, mode: modeList}
	driveToSettings(t, m, 4)
	typeRunes(m, "test-import.csv")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// 8 data rows, the github/alice duplicate is skipped -> 7 entries.
	if !strings.Contains(m.status, "Imported 7 entries") {
		t.Fatalf("unexpected status: %q", m.status)
	}
	if len(m.entries) != 7 {
		t.Fatalf("entries = %d, want 7", len(m.entries))
	}
	byService := map[string]vault.Entry{}
	for _, e := range m.entries {
		byService[e.Service] = e
	}
	checks := []struct {
		svc, login, pass, totp string
	}{
		{"github", "alice", "T3st!Pass#", "JBSWY3DPEHPK3PXP"},
		{"mail.ru", "ivan", "Пароль, с запятой", ""},
		{"bank.example", "olga", `Сложный "кавычки" пароль`, ""},
		{"old-forum", "", "nopassword123", ""},
		{"server", "root", "p@$$w0rd!#%^&*()", ""},
		{"spaces-test", "user", "  leading and trailing  ", ""},
		{"тест-сервис", "пользователь", "пароль123", ""},
	}
	for _, c := range checks {
		e, ok := byService[c.svc]
		if !ok {
			t.Fatalf("service %q missing", c.svc)
		}
		if e.Login != c.login || string(e.Password) != c.pass || string(e.TotpSecret) != c.totp {
			t.Fatalf("entry %q mismatch: login=%q pass=%q totp=%q", c.svc, e.Login, e.Password, e.TotpSecret)
		}
	}
	// Persisted vault must round-trip.
	reloaded, err := vault.Load(path, master)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 7 {
		t.Fatalf("vault on disk has %d entries", len(reloaded))
	}
}
