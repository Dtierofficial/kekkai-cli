package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kekkai/vault"
)

func TestFormEscapeRestoresListShortcuts(t *testing.T) {
	for _, mode := range []tuiMode{modeAdd, modeSettings} {
		m := &tuiModel{mode: mode, field: 3, settingsField: 2, service: "service", login: "login", password: []byte("password"), totpSecret: []byte("secret"), newMaster: []byte("master")}
		m.key(tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != modeList || m.field != 0 || m.settingsField != 0 || m.editing || m.service != "" || m.login != "" || len(m.password) != 0 || len(m.totpSecret) != 0 || len(m.newMaster) != 0 {
			t.Fatalf("form state was not reset for mode %d: %#v", mode, m)
		}
		m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		if m.mode != modeAdd || m.field != 0 {
			t.Fatalf("list shortcut a did not reactivate after mode %d", mode)
		}
		vault.Zero(m.password)
	}
}

func TestBackgroundMessagesDoNotBreakListShortcuts(t *testing.T) {
	m := &tuiModel{mode: modeList, entries: []vault.Entry{{Service: "svc", Login: "user", Password: []byte("pass")}}, lastActivity: time.Now(), clipboardID: 1, totpID: 1}
	for i := 0; i < 500; i++ {
		m.Update(tickMsg(time.Now()))
		m.Update(clipboardHideMsg{id: 0})
		m.Update(totpTickMsg{id: 0})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		if m.mode == modeSettings {
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		}
		if m.mode != modeList {
			t.Fatalf("iteration %d left list mode: %v", i, m.mode)
		}
	}
}

func TestWindowLifecycleMessagesPreserveState(t *testing.T) {
	m := &tuiModel{mode: modeList, field: 0, entries: []vault.Entry{{Service: "svc"}}, selected: 0}
	before := m.mode
	for i := 0; i < 100; i++ {
		m.Update(tea.BlurMsg{})
		m.Update(tea.FocusMsg{})
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	}
	if m.mode != before || m.field != 0 || len(m.entries) != 1 {
		t.Fatalf("window lifecycle events changed model state: %#v", m)
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if model.(*tuiModel).mode != modeSettings {
		t.Fatal("key input stopped after focus/blur events")
	}
}

func TestUnknownLifecycleNoiseDoesNotBlockKeys(t *testing.T) {
	m := &tuiModel{mode: modeList, entries: []vault.Entry{{Service: "svc"}}}
	type terminalNoise struct{ value string }
	for i := 0; i < 100; i++ {
		m.Update(terminalNoise{value: "focus/resize noise"})
		m.Update(tea.BlurMsg{})
		m.Update(terminalNoise{value: "unknown CSI"})
		m.Update(tea.FocusMsg{})
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.mode != modeSettings {
		t.Fatal("key input stopped after unknown lifecycle messages")
	}
}

func TestStaleTOTPTicksCannotCreateAnotherTimer(t *testing.T) {
	m := &tuiModel{mode: modeList, totpID: 4, totpSecret: []byte("JBSWY3DPEHPK3PXP")}
	model, cmd := m.Update(totpTickMsg{id: 3})
	if cmd != nil || model.(*tuiModel).totpCode != "" {
		t.Fatal("stale TOTP tick changed the model")
	}
	model, cmd = m.Update(totpTickMsg{id: 4})
	if cmd == nil || model.(*tuiModel).totpCode == "" {
		t.Fatal("active TOTP tick was not scheduled")
	}
	// The command captures the immutable session id, not the mutable model id.
	m.totpID = 5
	if msg, ok := cmd().(totpTickMsg); !ok || msg.id != 4 {
		t.Fatalf("timer captured mutable ID: %#v", msg)
	}
}

func TestSettingsCursorReachesLastItem(t *testing.T) {
	m := &tuiModel{mode: modeSettings}
	for i := 0; i < settingsItemCount+3; i++ {
		m.settingsKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.settingsField != maxSettingsIndex() {
		t.Fatalf("cursor stopped at %d, want %d", m.settingsField, maxSettingsIndex())
	}
	m.settingsMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.settingsField != maxSettingsIndex() {
		t.Fatalf("mouse moved cursor past last item: %d", m.settingsField)
	}
	model, _ := m.settingsKey(tea.KeyMsg{Type: tea.KeyEnter})
	if model.(*tuiModel).mode != modeUnsafeExport {
		t.Fatalf("last item did not open unsafe export: %v", model.(*tuiModel).mode)
	}
}

func TestReturnToListClearsFormAfterSavePath(t *testing.T) {
	m := &tuiModel{mode: modeAdd, field: 3, service: "service", login: "login", password: []byte("password"), totpSecret: []byte("secret"), newMaster: []byte("master"), editing: true, editIndex: 4}
	m.returnToList()
	if m.mode != modeList || m.field != 0 || m.editing || len(m.password) != 0 || len(m.totpSecret) != 0 || len(m.newMaster) != 0 {
		t.Fatalf("returnToList left stale form state: %#v", m)
	}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
}

func TestTOTPFieldEscapeRestoresAllListShortcuts(t *testing.T) {
	for _, shortcut := range []rune{'a', 'd', 'e', 'c', 't', 's', 'q'} {
		m := &tuiModel{mode: modeAdd, field: 3, totpSecret: []byte("JBSWY3DPEHPK3PXP"), password: []byte("unchanged")}
		m.key(tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != modeList || m.field != 0 || m.editing || len(m.totpSecret) != 0 || len(m.password) != 0 {
			t.Fatalf("TOTP form did not blur on Esc for %q", shortcut)
		}
		m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{shortcut}})
		if shortcut == 'a' && m.mode != modeAdd {
			t.Fatalf("shortcut %q was swallowed after TOTP form", shortcut)
		}
		if shortcut == 's' && m.mode != modeSettings {
			t.Fatalf("shortcut %q was swallowed after TOTP form", shortcut)
		}
	}
}

// TestFailedUnlockDoesNotPoisonMasterBuffer reproduces the reported freeze:
// after the idle timer locks the session, stray list hotkeys ('a', 'c', 'e',
// 'd', 's', 'q') and arrows are swallowed by the hidden master-password
// buffer while Enter still fires actions. A failed unlock must clear that
// buffer, otherwise every retry sends "garbage + password" and the vault can
// never be unlocked again without restarting the program.
func TestFailedUnlockDoesNotPoisonMasterBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.kek")
	if err := vault.New(path, []byte("correct-horse")); err != nil {
		t.Fatal(err)
	}
	m := &tuiModel{path: path, mode: modeLogin, locked: true}
	for _, r := range []rune{'a', 'c', 'e', 'd', 's', 'q'} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.status != "Invalid master password" {
		t.Fatalf("unexpected status after wrong password: %q", m.status)
	}
	if len(m.master) != 0 {
		t.Fatalf("failed unlock left poisoned master buffer: %q", m.master)
	}
	for _, r := range "correct-horse" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeList {
		t.Fatalf("correct password rejected after a failed attempt: mode=%v status=%q", m.mode, m.status)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.mode != modeAdd {
		t.Fatal("hotkey 'a' is dead immediately after unlock")
	}
}

// TestSearchModeKeepsNavigationAndEscapeAlive reproduces the trap where the
// search overlay swallows every hotkey and both arrow keys, leaving Enter as
// the only responsive key.
func TestSearchModeKeepsNavigationAndEscapeAlive(t *testing.T) {
	m := &tuiModel{mode: modeList, entries: []vault.Entry{
		{Service: "alpha", Login: "u1"},
		{Service: "alpine", Login: "u2"},
		{Service: "beta", Login: "u3"},
	}}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searching {
		t.Fatal("'/'' did not enter search mode")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.search != "al" {
		t.Fatalf("search query = %q, want %q", m.search, "al")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selected != 1 {
		t.Fatalf("arrow down is dead while searching: selected=%d", m.selected)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selected != 0 {
		t.Fatalf("arrow up is dead while searching: selected=%d", m.selected)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.search != "" {
		t.Fatalf("Esc did not clear search state: searching=%v query=%q", m.searching, m.search)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.mode != modeSettings {
		t.Fatal("hotkey 's' is dead after leaving search mode")
	}
}

// TestClipboardClearTimerSurvivesKeypresses guards the race between the
// background clipboard helper and user input: unrelated keypresses must not
// invalidate the pending clipboard-clear timer.
func TestClipboardClearTimerSurvivesKeypresses(t *testing.T) {
	m := &tuiModel{mode: modeList, clipboardDuration: 35 * time.Second, entries: []vault.Entry{{Service: "svc", Login: "u", Password: []byte("p")}}}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatal("copy command missing")
	}
	id := m.clipboardID
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m.Update(tickMsg(time.Now()))
	_, clearTimer := m.Update(clipboardCopiedMsg{id: id})
	if clearTimer == nil {
		t.Fatal("clipboard clear timer was cancelled by unrelated keypresses")
	}
}

// TestEveryModalModeEscapesToListAndRestoresHotkeys is the escapability
// invariant: whatever state the model degrades into, Esc must return it to
// the list and list shortcuts must reactivate.
func TestEveryModalModeEscapesToListAndRestoresHotkeys(t *testing.T) {
	for _, mode := range []tuiMode{modeAdd, modeConfirmDelete, modeSettings, modeImport, modeUnsafeExport} {
		m := &tuiModel{mode: mode, entries: []vault.Entry{{Service: "svc"}}, selected: 0}
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != modeList {
			t.Fatalf("mode %d did not escape to list via Esc: %d", mode, m.mode)
		}
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		if m.mode != modeAdd {
			t.Fatalf("hotkey 'a' dead after escaping mode %d", mode)
		}
	}
}

// TestListEscClearsFilterBeforeQuitting ensures a leftover filter never
// makes Esc feel unresponsive: the first Esc clears the filter, only the
// second quits.
func TestListEscClearsFilterBeforeQuitting(t *testing.T) {
	m := &tuiModel{mode: modeList, entries: []vault.Entry{{Service: "alpha"}}, search: "zz"}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.search != "" || m.quitting {
		t.Fatalf("Esc with active filter must clear the filter: search=%q quitting=%v", m.search, m.quitting)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.quitting {
		t.Fatal("second Esc must quit")
	}
}

// TestRussianLayoutHotkeysStayAlive reproduces the reported "all keys dead
// after Alt+Tab except Enter": the Alt+Tab gesture fires the system
// Alt+Shift layout switch, so the console ends up in the Russian layout and
// pressing 'a' delivers 'ф'. Hotkeys must match the physical key's Latin
// meaning regardless of the active layout.
func TestRussianLayoutHotkeysStayAlive(t *testing.T) {
	m := &tuiModel{mode: modeList, entries: []vault.Entry{{Service: "svc", Login: "u", Password: []byte("p")}}, lastActivity: time.Now()}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ф'}}) // physical A in RU layout
	if m.mode != modeAdd {
		t.Fatalf("RU 'ф' did not open the add form: mode=%d", m.mode)
	}
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'ы'}}) // physical S in RU layout
	if m.mode != modeSettings {
		t.Fatalf("RU 'ы' did not open settings: mode=%d", m.mode)
	}
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'в'}}) // physical D in RU layout
	if m.mode != modeConfirmDelete {
		t.Fatalf("RU 'в' did not open delete confirm: mode=%d", m.mode)
	}
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'т'}}) // physical N in RU layout
	if m.mode != modeList {
		t.Fatalf("RU 'т' did not cancel delete: mode=%d", m.mode)
	}
}

// TestNulRuneDoesNotPoisonBuffers guards against the NUL rune key event
// that a layout switch can emit: it must never reach password or text
// buffers.
func TestNulRuneDoesNotPoisonBuffers(t *testing.T) {
	m := &tuiModel{mode: modeLogin, path: filepath.Join(t.TempDir(), "vault.kek")}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}})
	if len(m.master) != 0 {
		t.Fatalf("NUL rune reached the master buffer: %q", m.master)
	}
	m2 := &tuiModel{mode: modeAdd, field: 0}
	m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}})
	if m2.service != "" {
		t.Fatalf("NUL rune reached the service buffer: %q", m2.service)
	}
}

// TestHotkeyLayoutAndCaseSymmetry pins the input symmetry contract: every
// hotkey must fire identically for Latin and Russian (ЙЦУКЕН) layouts and
// for lower/upper case (Shift, CapsLock).
func TestHotkeyLayoutAndCaseSymmetry(t *testing.T) {
	cases := []struct {
		name  string
		runes []rune
		check func(*tuiModel) string
		want  string
	}{
		{"s en", []rune{'s'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeSettings) }, "true"},
		{"S en upper", []rune{'S'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeSettings) }, "true"},
		{"ы ru", []rune{'ы'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeSettings) }, "true"},
		{"Ы ru upper", []rune{'Ы'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeSettings) }, "true"},
		{"a en", []rune{'a'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeAdd) }, "true"},
		{"A en upper", []rune{'A'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeAdd) }, "true"},
		{"ф ru", []rune{'ф'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeAdd) }, "true"},
		{"Ф ru upper", []rune{'Ф'}, func(m *tuiModel) string { return fmt.Sprint(m.mode == modeAdd) }, "true"},
		{"/ en", []rune{'/'}, func(m *tuiModel) string { return fmt.Sprint(m.searching) }, "true"},
		{". ru physical slash", []rune{'.'}, func(m *tuiModel) string { return fmt.Sprint(m.searching) }, "true"},
	}
	for _, tc := range cases {
		m := &tuiModel{mode: modeList, entries: []vault.Entry{{Service: "svc", Login: "u", Password: []byte("p")}}}
		m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: tc.runes})
		if got := tc.check(m); got != tc.want {
			t.Fatalf("%s: got %s", tc.name, got)
		}
	}
}

// TestConfirmYesSymmetryAcrossLayoutsAndCase checks the y/N confirmations:
// 'y', 'Y', RU 'н' (physical y) and RU 'Н' must all confirm.
func TestConfirmYesSymmetryAcrossLayoutsAndCase(t *testing.T) {
	for _, r := range []rune{'y', 'Y', 'н', 'Н'} {
		m := &tuiModel{mode: modeConfirmDelete, entries: []vault.Entry{{Service: "svc"}}, selected: 0}
		m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if m.mode != modeList {
			t.Fatalf("rune %q did not confirm deletion: mode=%d", r, m.mode)
		}
	}
}

// TestGTypesIntoPasswordFieldAndCtrlGGenerates guards the password field
// hotkey conflict: the literal 'g' rune must be typed into the password,
// and only Ctrl+G may trigger password generation.
func TestGTypesIntoPasswordFieldAndCtrlGGenerates(t *testing.T) {
	typing := &tuiModel{mode: modeAdd, field: 2}
	typing.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if string(typing.password) != "g" {
		t.Fatalf("'g' was not typed into the password field: %q", typing.password)
	}
	prefix := &tuiModel{mode: modeAdd, field: 2}
	for _, r := range "garbage9" {
		prefix.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if string(prefix.password) != "garbage9" {
		t.Fatalf("password containing 'g' was mangled: %q", prefix.password)
	}

	gen := &tuiModel{mode: modeAdd, field: 2}
	model, _ := gen.key(tea.KeyMsg{Type: tea.KeyCtrlG})
	gm := model.(*tuiModel)
	if len(gm.password) < 16 {
		t.Fatalf("Ctrl+G did not generate a password: %q", gm.password)
	}
	if gm.strength == "" {
		t.Fatal("Ctrl+G did not update the strength indicator")
	}
	// Generation must replace the current draft, not append to it.
	if len(gm.password) != 18 {
		t.Fatalf("unexpected generated length: %d", len(gm.password))
	}
}

// TestHTypesIntoPasswordFieldAndCtrlHChecksHIBP guards the same hotkey
// conflict for 'h': the literal rune must be typed, and only Ctrl+H may
// trigger the HIBP check.
func TestHTypesIntoPasswordFieldAndCtrlHChecksHIBP(t *testing.T) {
	typing := &tuiModel{mode: modeAdd, field: 2}
	typing.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if string(typing.password) != "h" {
		t.Fatalf("'h' was not typed into the password field: %q", typing.password)
	}
	prefix := &tuiModel{mode: modeAdd, field: 2}
	for _, r := range "hunter2" {
		prefix.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if string(prefix.password) != "hunter2" {
		t.Fatalf("password containing 'h' was mangled: %q", prefix.password)
	}

	check := &tuiModel{mode: modeAdd, field: 2, password: []byte("correct-horse")}
	model, cmd := check.key(tea.KeyMsg{Type: tea.KeyCtrlH})
	if cmd == nil {
		t.Fatal("Ctrl+H did not start the HIBP check")
	}
	if model.(*tuiModel).status != "" {
		t.Fatalf("Ctrl+H must not set status synchronously: %q", model.(*tuiModel).status)
	}
	empty := &tuiModel{mode: modeAdd, field: 2}
	if _, cmd := empty.key(tea.KeyMsg{Type: tea.KeyCtrlH}); cmd != nil {
		t.Fatal("Ctrl+H with an empty password must not start a check")
	}
}
