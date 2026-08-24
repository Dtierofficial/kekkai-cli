package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"kekkai/vault"
)

// driveToSettings unlocks a model, opens settings and walks the cursor down
// to the requested row using real key events.
func driveToSettings(t *testing.T, m *tuiModel, row int) {
	t.Helper()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.mode != modeSettings {
		t.Fatalf("'s' did not open settings: mode=%d", m.mode)
	}
	for i := 0; i < row; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.settingsField != row {
		t.Fatalf("cursor at %d, want %d", m.settingsField, row)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// typeRunes feeds a string into the model one key event per rune.
func typeRunes(m *tuiModel, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// TestEncryptedBackupFromSettings covers Settings -> Export Backup (.enc):
// the backup must be a byte copy of the vault that vault.Load accepts with
// the same master password.
func TestEncryptedBackupFromSettings(t *testing.T) {
	tmp := t.TempDir()
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
	entries := []vault.Entry{{Service: "svc", Login: "user", Password: []byte("p@ss,word")}}
	if err := vault.Save(path, master, entries); err != nil {
		t.Fatal(err)
	}

	m := &tuiModel{path: path, master: append([]byte(nil), master...), entries: entries, mode: modeList, selected: 0}
	driveToSettings(t, m, 5)

	if m.status == "" || !strings.Contains(m.status, "Encrypted backup saved to") {
		t.Fatalf("unexpected status after backup: %q", m.status)
	}
	name := strings.TrimPrefix(m.status, "Encrypted backup saved to ")
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	orig, _ := os.ReadFile(path)
	if string(b) != string(orig) {
		t.Fatal("backup is not a copy of the vault file")
	}
	restored, err := vault.Load(name, master)
	if err != nil {
		t.Fatalf("backup does not load: %v", err)
	}
	if len(restored) != 1 || restored[0].Service != "svc" || string(restored[0].Password) != "p@ss,word" {
		t.Fatalf("restored entries mismatch: %#v", restored)
	}
	// Staying in settings after a backup is intended: the status reports
	// the result and the user may adjust more options.
	if m.mode != modeSettings {
		t.Fatalf("backup flow changed mode: mode=%d", m.mode)
	}
}

// TestUnsafeExportAndReimportRoundtrip covers Settings -> Export to CSV
// (Unsafe) and Settings -> Import from CSV: a kekkai export must reimport
// losslessly, including passwords with CSV-special characters and TOTP
// secrets, while duplicate rows are skipped.
func TestUnsafeExportAndReimportRoundtrip(t *testing.T) {
	tmp := t.TempDir()
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
	entries := []vault.Entry{
		{Service: "svc", Login: "user", Password: []byte(`p@ss "quoted", with; specials`)},
		{Service: "totp-svc", Login: "u2", Password: []byte("pw2"), TotpSecret: []byte("JBSWY3DPEHPK3PXP")},
	}
	if err := vault.Save(path, master, entries); err != nil {
		t.Fatal(err)
	}

	m := &tuiModel{path: path, master: append([]byte(nil), master...), entries: entries, mode: modeList, selected: 0}

	// Export: settings row 6, then confirm with 'y'.
	driveToSettings(t, m, 6)
	if m.mode != modeUnsafeExport {
		t.Fatalf("export row did not open unsafe confirm: mode=%d", m.mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.mode != modeList {
		t.Fatalf("unsafe export did not return to list: mode=%d", m.mode)
	}
	if _, err := os.Stat("kekkai-export.csv"); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	// Export content check: header + both rows, special chars intact.
	f, err := os.Open("kekkai-export.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(f).ReadAll()
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0][0] != "service" || rows[1][2] != `p@ss "quoted", with; specials` || rows[2][3] != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("unexpected CSV content: %#v", rows)
	}

	// Reimport: settings row 4, type the path, press Enter.
	m2 := &tuiModel{path: path, master: append([]byte(nil), master...), entries: []vault.Entry{{Service: "keep", Login: "me", Password: []byte("keep-pw")}}, mode: modeList}
	driveToSettings(t, m2, 4)
	if m.mode != modeList || m2.mode != modeImport {
		t.Fatalf("import row did not open import screen: mode=%d", m2.mode)
	}
	typeRunes(m2, "kekkai-export.csv")
	m2.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m2.status, "Imported 2 entries") {
		t.Fatalf("unexpected import status: %q", m2.status)
	}
	if m2.mode != modeList {
		t.Fatalf("import did not return to list: mode=%d", m2.mode)
	}
	if len(m2.entries) != 3 {
		t.Fatalf("merged entries = %d, want 3 (existing + 2 imported)", len(m2.entries))
	}
	var found int
	for _, e := range m2.entries {
		if e.Service == "totp-svc" && string(e.TotpSecret) == "JBSWY3DPEHPK3PXP" {
			found++
		}
		if e.Service == "svc" && string(e.Password) != `p@ss "quoted", with; specials` {
			t.Fatalf("password corrupted by roundtrip: %q", e.Password)
		}
	}
	if found != 1 {
		t.Fatal("TOTP secret lost in roundtrip")
	}

	// Persisted vault must contain the merge too.
	reloaded, err := vault.Load(path, master)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 3 {
		t.Fatalf("vault on disk has %d entries, want 3", len(reloaded))
	}

	// Importing the same file again must skip duplicates.
	m3 := &tuiModel{path: path, master: append([]byte(nil), master...), entries: m2.entries, mode: modeList}
	driveToSettings(t, m3, 4)
	typeRunes(m3, "kekkai-export.csv")
	m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m3.status, "Imported 0 entries") {
		t.Fatalf("duplicates were not skipped: %q", m3.status)
	}
}

// TestImportForeignCSVFormat verifies Bitwarden/Chrome-style headers
// (url/name + username + password) are accepted, rows without service or
// password are skipped, and a broken path fails gracefully.
func TestImportForeignCSVFormat(t *testing.T) {
	tmp := t.TempDir()
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prevWd)

	foreign := "name,username,password,extra\ngithub,alice,gh-pass,x\n,skipme,nopass,y\nurlless,,empty-pw,z\n"
	if err := os.WriteFile("foreign.csv", []byte(foreign), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "vault.kek")
	master := []byte("master-pass")
	if err := vault.New(path, master); err != nil {
		t.Fatal(err)
	}

	m := &tuiModel{path: path, master: append([]byte(nil), master...), entries: nil, mode: modeList}
	driveToSettings(t, m, 4)
	typeRunes(m, "foreign.csv")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Rows without service or password are skipped; an empty login is
	// tolerated on import (unlike the add form) because foreign exports
	// legitimately contain password-only entries.
	if !strings.Contains(m.status, "Imported 2 entries") {
		t.Fatalf("unexpected status: %q", m.status)
	}
	if len(m.entries) != 2 {
		t.Fatalf("unexpected imported entries: %#v", m.entries)
	}
	if m.entries[0].Service != "github" || m.entries[0].Login != "alice" || string(m.entries[0].Password) != "gh-pass" {
		t.Fatalf("first entry mismatch: %#v", m.entries[0])
	}
	if m.entries[1].Service != "urlless" || m.entries[1].Login != "" || string(m.entries[1].Password) != "empty-pw" {
		t.Fatalf("second entry mismatch: %#v", m.entries[1])
	}

	// A nonexistent path must fail with a status, not crash or wipe state.
	m2 := &tuiModel{path: path, master: append([]byte(nil), master...), entries: m.entries, mode: modeList}
	driveToSettings(t, m2, 4)
	typeRunes(m2, "no-such-file.csv")
	m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m2.status, "Import failed") {
		t.Fatalf("missing file did not fail gracefully: %q", m2.status)
	}
	if m2.mode != modeImport {
		t.Fatalf("failed import should keep the import screen: mode=%d", m2.mode)
	}
	if len(m2.entries) != 2 {
		t.Fatalf("failed import changed entries: %d", len(m2.entries))
	}
	// A quoted path (Explorer "Copy as path") must import too: re-importing
	// foreign.csv yields 0 new entries (all duplicates).
	m4 := &tuiModel{path: path, master: append([]byte(nil), master...), entries: m.entries, mode: modeList}
	driveToSettings(t, m4, 4)
	typeRunes(m4, `"foreign.csv"`)
	m4.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m4.status, "Imported 0 entries") {
		t.Fatalf("quoted path was not accepted: %q", m4.status)
	}
}

// TestImportRejectsOversizedCSV ensures a CSV beyond the entry cap fails
// with an error instead of exhausting memory.
func TestImportRejectsOversizedCSV(t *testing.T) {
	tmp := t.TempDir()
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(prevWd)

	var b strings.Builder
	b.WriteString("name,username,password\n")
	for i := 0; i < 10001; i++ {
		b.WriteString("svc" + strconv.Itoa(i) + ",login,pass\n")
	}
	if err := os.WriteFile("huge.csv", []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmp, "vault.kek")
	master := []byte("master-pass")
	if err := vault.New(path, master); err != nil {
		t.Fatal(err)
	}

	m := &tuiModel{path: path, master: append([]byte(nil), master...), entries: nil, mode: modeList}
	driveToSettings(t, m, 4)
	typeRunes(m, "huge.csv")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.status, "too many entries") {
		t.Fatalf("oversized CSV was not rejected: %q", m.status)
	}
	if len(m.entries) > 10000 {
		t.Fatalf("entries beyond the cap were kept: %d", len(m.entries))
	}
}

// TestListCursorClampsToEntries pins the panic guard: a stale cursor beyond
// the entry slice must clamp instead of indexing out of range when list
// hotkeys fire.
func TestListCursorClampsToEntries(t *testing.T) {
	m := &tuiModel{mode: modeList, selected: 7, entries: []vault.Entry{{Service: "only", Login: "u", Password: []byte("p")}}}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !m.editing || m.editIndex != 0 {
		t.Fatalf("stale cursor broke the edit flow: editing=%v editIndex=%d", m.editing, m.editIndex)
	}
	if string(m.password) != "p" || m.service != "only" {
		t.Fatalf("wrong entry loaded into the form: %q %q", m.service, m.password)
	}
	m.returnToList()
	m.selected = 99
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // reveal must not panic
	if !m.revealed {
		t.Fatal("reveal stopped working after cursor clamp")
	}
}
