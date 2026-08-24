package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kekkai/vault"
)

var (
	fox      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8A3D")).Bold(true)
	coral    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5C5C"))
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("#777B8A"))
	soft     = lipgloss.NewStyle().Foreground(lipgloss.Color("#C6CAD6"))
	selected = lipgloss.NewStyle().Foreground(lipgloss.Color("#17131A")).Background(lipgloss.Color("#FF8A3D")).Bold(true).Padding(0, 1)
	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#44313A")).Padding(1, 2)
	keyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8A3D")).Bold(true)
)

const banner = `
   /\   /\
  /  \_/  \     K E K K A I
  \       /     secure password vault
   \_____/
`

type tuiMode byte

const settingsItemCount = 7

func maxSettingsIndex() int { return settingsItemCount - 1 }

const (
	modeLogin tuiMode = iota
	modeList
	modeAdd
	modeConfirmDelete
	modeSettings
	modeImport
	modeUnsafeExport
)

type tuiModel struct {
	path              string
	master            []byte
	entries           []vault.Entry
	selected          int
	mode              tuiMode
	field             int
	service           string
	login             string
	password          []byte
	revealed          bool
	status            string
	quitting          bool
	search            string
	searching         bool
	editing           bool
	editIndex         int
	strength          string
	lastActivity      time.Time
	lockDuration      time.Duration
	clipboardDuration time.Duration
	totpSecret        []byte
	totpCode          string
	totpSeconds       int
	newMaster         []byte
	settingsField     int
	theme             Theme
	locked            bool
	clipboardID       uint64
	totpID            uint64
	transferPath      string
}

func interactive() error {
	path, err := vaultPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("vault does not exist; run 'kekkai init' first")
		}
		return err
	}
	theme := loadTheme()
	m := tuiModel{path: path, mode: modeLogin, theme: theme, lastActivity: time.Now(), lockDuration: idleTimeout, clipboardDuration: 35 * time.Second}
	restoreConsole, err := SetupConsole()
	if err != nil {
		return err
	}
	defer restoreConsole()
	// Input stays on the default path (stdin, Bubble Tea's coninput
	// reader): it was verified with a minimal Bubble Tea program that this
	// path survives Alt+Tab focus loss, while custom console input setups
	// (VT input mode, a dedicated CONIN$ handle) broke key delivery.
	p := tea.NewProgram(&m, tea.WithAltScreen())
	_, err = p.Run()
	vault.Zero(m.master)
	clearEntries(m.entries)
	vault.Zero(m.password)
	return err
}

func (m *tuiModel) Init() tea.Cmd { return tick() }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if f := debugLog(); f != nil {
		fmt.Fprintf(f, "%s msg=%T mode=%d searching=%v status=%q\n", time.Now().Format("15:04:05.000"), msg, m.mode, m.searching, m.status)
	}
	switch msg := msg.(type) {
	case tickMsg:
		if m.mode != modeLogin && m.lockDuration > 0 && time.Since(m.lastActivity) >= m.lockDuration {
			clearEntries(m.entries)
			m.entries = nil
			vault.Zero(m.master)
			m.master = nil
			vault.Zero(m.password)
			m.password = nil
			vault.Zero(m.totpSecret)
			m.totpSecret = nil
			m.totpCode = ""
			vault.Zero(m.newMaster)
			m.newMaster = nil
			m.service, m.login = "", ""
			m.editing = false
			m.mode = modeLogin
			m.locked = true
			m.selected = 0
			m.revealed = false
			m.search = ""
			m.searching = false
			m.status = "Session locked"
		}
		return m, tick()
	case clipboardCopiedMsg:
		if msg.id != m.clipboardID {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Clipboard unavailable"
			return m, nil
		}
		m.status = m.clipboardStatus(msg.message)
		return m, tea.Tick(m.clipboardDuration, func(time.Time) tea.Msg { return clipboardClearDueMsg{id: msg.id} })
	case totpTickMsg:
		if msg.id != m.totpID || len(m.totpSecret) == 0 || m.mode != modeList {
			return m, nil
		}
		if len(m.totpSecret) > 0 {
			m.totpCode, m.totpSeconds, _ = totpCode(m.totpSecret, time.Now())
		}
		id := msg.id
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return totpTickMsg{id: id} })
	case clipboardClearDueMsg:
		if msg.id != m.clipboardID {
			return m, nil
		}
		return m, clearClipboardCmd(msg.id)
	case clipboardClearedMsg:
		if msg.id != m.clipboardID {
			return m, nil
		}
		if msg.err != nil {
			m.status = "Clipboard clear failed"
			return m, nil
		}
		m.status = "Clipboard cleared"
		return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clipboardHideMsg{id: msg.id} })
	case clipboardHideMsg:
		if msg.id != m.clipboardID {
			return m, nil
		}
		m.status = ""
		return m, nil
	case hibpMsg:
		if msg.err != nil {
			m.status = "HIBP check failed: " + msg.err.Error()
		} else if msg.found > 0 {
			m.status = fmt.Sprintf("Warning: password found %d time(s) in breaches", msg.found)
		} else {
			m.status = "Password not found in HIBP"
		}
		return m, nil
	case tea.MouseMsg:
		m.lastActivity = time.Now()
		m.status = ""
		return m.mouse(msg)
	case tea.FocusMsg, tea.BlurMsg, tea.WindowSizeMsg:
		// Terminal lifecycle events are metadata only. Never reset mode, input
		// buffers, selection, or activity state when a window is minimized,
		// restored, or resized.
		return m, nil
	case tea.KeyMsg:
		if msg.Type == tea.KeyRunes {
			// A layout switch (Alt+Shift during Alt+Tab) can emit a
			// key event with a NUL rune; it must never reach the
			// password/text buffers.
			clean := msg.Runes[:0]
			for _, r := range msg.Runes {
				if r != 0 {
					clean = append(clean, r)
				}
			}
			if len(clean) == 0 {
				return m, nil
			}
			msg.Runes = clean
		}
		m.lastActivity = time.Now()
		m.status = ""
		// NOTE: keypresses must not invalidate the clipboard session id.
		// Copy commands capture the id when they start; incrementing here
		// would cancel the pending clipboard-clear timer whenever the user
		// types while the clipboard helper process is still running.
		return m.key(msg)
	}
	// Bubble Tea v1.3.4 keeps unknown ANSI input in unexported message
	// types. Ignore every unrecognized message here; terminal lifecycle
	// noise must never alter focus, mode, or block the next KeyMsg.
	return m, nil
}

// ruToEn maps Cyrillic runes produced by the standard Russian (ЙЦУКЕН)
// keyboard layout to the Latin rune produced by the same physical key in
// the US layout. Alt+Tab gestures frequently trigger the system Alt+Shift
// layout switch, so after returning from another window the console may be
// in Russian layout: pressing 'a' delivers 'ф' and every hotkey appears
// dead. Hotkeys are therefore matched through hotkeyRune, which normalizes
// the rune to the physical key's Latin meaning.
var ruToEn = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p',
	'х': '[', 'ъ': ']', 'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l',
	'ж': ';', 'э': '\'', 'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.', 'ё': '`',
	// The physical '/' key produces '.' in the Russian layout, so that
	// rune must also reach the search hotkey.
	'.': '/',
}

// hotkeyRune returns the lowercase Latin hotkey rune for a physical key
// regardless of the active keyboard layout and Shift/CapsLock state.
func hotkeyRune(r rune) rune {
	lower := unicode.ToLower(r)
	if en, ok := ruToEn[lower]; ok {
		return en
	}
	return lower
}

// trimLastRune removes the last UTF-8 rune from b. A byte-level trim would
// corrupt multibyte input: a Backspace in a Cyrillic master password could
// leave a dangling byte, and the vault would then be saved with a password
// the user can never retype - a permanent lockout.
func trimLastRune(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	_, n := utf8.DecodeLastRune(b)
	if n <= 0 || n > len(b) {
		n = 1
	}
	return b[:len(b)-n]
}

func (m *tuiModel) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if f := debugLog(); f != nil {
		fmt.Fprintf(f, "%s key=%v runes=%q\n", time.Now().Format("15:04:05.000"), msg.Type, msg.Runes)
	}
	if m.mode == modeLogin {
		return m.loginKey(msg)
	}
	if m.mode == modeAdd {
		return m.addKey(msg)
	}
	if m.mode == modeConfirmDelete {
		return m.deleteKey(msg)
	}
	if m.mode == modeSettings {
		return m.settingsKey(msg)
	}
	if m.mode == modeImport || m.mode == modeUnsafeExport {
		return m.transferKey(msg)
	}
	// Defensive focus normalization: no form buffer may intercept list
	// shortcuts once the model is back on the main screen.
	if m.mode == modeList && (m.field != 0 || m.editing || len(m.newMaster) > 0) {
		m.returnToList()
	}
	// Keep the cursor inside the entry slice: every list action below
	// indexes m.entries[m.selected], so an out-of-range cursor would panic
	// the whole manager.
	if m.selected < 0 || m.selected >= len(m.entries) {
		m.selected = 0
	}
	if m.searching {
		switch msg.Type {
		case tea.KeyEsc:
			m.searching = false
			m.search = ""
			return m, nil
		case tea.KeyEnter:
			m.searching = false
			return m, nil
		case tea.KeyBackspace:
			if len(m.search) > 0 {
				_, n := utf8.DecodeLastRuneInString(m.search)
				m.search = m.search[:len(m.search)-n]
			}
			return m, nil
		case tea.KeyRunes:
			m.search += string(msg.Runes)
			return m, nil
		case tea.KeyUp, tea.KeyDown, tea.KeyCtrlC:
			// Navigation and interrupt must never be trapped by the search
			// overlay: fall through to the shared list key handling below.
		default:
			return m, nil
		}
	}
	indices := m.visibleIndices()
	position := 0
	for i, index := range indices {
		if index == m.selected {
			position = i
			break
		}
	}
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		if msg.Type == tea.KeyEsc && m.search != "" {
			// A leftover filter must never make Esc quit instantly: clear it
			// first so the hotkeys feel responsive again.
			m.search = ""
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	case tea.KeyUp:
		if position > 0 {
			m.selected = indices[position-1]
			m.clearTOTP()
		}
	case tea.KeyDown:
		if position < len(indices)-1 {
			m.selected = indices[position+1]
			m.clearTOTP()
		}
	case tea.KeyEnter:
		if len(m.entries) > 0 && len(indices) > 0 {
			m.revealed = !m.revealed
		}
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch hotkeyRune(msg.Runes[0]) {
			case 'a':
				m.mode, m.field, m.service, m.login = modeAdd, 0, "", ""
				vault.Zero(m.password)
				vault.Zero(m.totpSecret)
				vault.Zero(m.newMaster)
				m.password = nil
				vault.Zero(m.totpSecret)
				m.totpSecret = nil
				m.revealed = false
			case 'd':
				if len(m.entries) > 0 {
					m.mode = modeConfirmDelete
				}
			case 'e':
				if len(m.entries) > 0 {
					m.editing = true
					m.editIndex = m.selected
					m.service = m.entries[m.selected].Service
					m.login = m.entries[m.selected].Login
					vault.Zero(m.password)
					m.password = append([]byte(nil), m.entries[m.selected].Password...)
					vault.Zero(m.totpSecret)
					m.totpSecret = append([]byte(nil), m.entries[m.selected].TotpSecret...)
					m.strength = passwordStrength(m.password)
					m.mode, m.field = modeAdd, 1
				}
			case 'c':
				if len(m.entries) > 0 {
					m.clipboardID++
					return m, copyClipboardCmd(m.clipboardID, m.entries[m.selected].Password, "Password copied; clipboard clears in 35s")
				}
			case 'u':
				if len(m.entries) > 0 {
					m.clipboardID++
					return m, copyClipboardCmd(m.clipboardID, []byte(m.entries[m.selected].Login), "Username copied; clipboard clears in 35s")
				}
			case '/':
				m.searching = true
				m.search = ""
			case 'q':
				return m, tea.Quit
			case 's':
				m.mode = modeSettings
			case 't':
				if len(m.entries) == 0 {
					m.status = "No entries available"
				} else if len(m.entries[m.selected].TotpSecret) == 0 {
					m.status = "No TOTP secret for this service"
				} else {
					m.totpSecret = append([]byte(nil), m.entries[m.selected].TotpSecret...)
					var err error
					m.totpCode, m.totpSeconds, err = totpCode(m.totpSecret, time.Now())
					if err != nil {
						m.status = "Invalid TOTP secret: " + err.Error()
					}
					m.totpID++
					id := m.totpID
					m.clipboardID++
					return m, tea.Batch(
						copyClipboardCmd(m.clipboardID, []byte(m.totpCode), fmt.Sprintf("TOTP code copied (valid %ds)", m.totpSeconds)),
						tea.Tick(time.Second, func(time.Time) tea.Msg { return totpTickMsg{id: id} }),
					)
				}
			}
		}
	}
	return m, nil
}

// returnToList is the single focus boundary for all form screens. The TUI
// uses plain buffers rather than textinput.Model, so resetting these fields is
// the equivalent of blurring every input before handling list shortcuts.
func (m *tuiModel) returnToList() {
	m.mode = modeList
	m.field = 0
	m.editing = false
	m.editIndex = 0
	m.service = ""
	m.login = ""
	vault.Zero(m.password)
	m.password = nil
	vault.Zero(m.totpSecret)
	m.totpSecret = nil
	vault.Zero(m.newMaster)
	m.newMaster = nil
	m.settingsField = 0
	m.searching = false
	m.revealed = false
	m.totpCode = ""
	m.totpSeconds = 0
}

func (m *tuiModel) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeSettings {
		return m.settingsMouse(msg)
	}
	if m.mode != modeList || m.searching || len(m.entries) == 0 {
		return m, nil
	}
	indices := m.visibleIndices()
	position := 0
	for i, index := range indices {
		if index == m.selected {
			position = i
			break
		}
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if position > 0 {
			m.selected = indices[position-1]
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if position < len(indices)-1 {
			m.selected = indices[position+1]
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	// Banner occupies five lines; the list starts below its rounded border.
	row := msg.Y - 8
	if row < 0 || row >= len(indices) {
		return m, nil
	}
	m.selected = indices[row]
	m.revealed = false
	m.totpCode = ""
	vault.Zero(m.totpSecret)
	m.totpSecret = nil
	return m, nil
}

func (m *tuiModel) settingsMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelUp {
		if m.settingsField > 0 {
			m.settingsField--
		}
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.settingsField < maxSettingsIndex() {
			m.settingsField++
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	// Settings rows are rendered below the banner and panel title.
	// The settings card has a title and padding above the three selectable rows.
	// Keep the hit areas generous so terminal font/padding differences do not
	// make the controls appear unresponsive.
	switch {
	case msg.Y >= 10 && msg.Y <= 13:
		m.settingsField = 0
	case msg.Y >= 14 && msg.Y <= 16:
		m.settingsField = 1
	case msg.Y >= 17 && msg.Y <= 20:
		m.settingsField = 2
	case msg.Y >= 21 && msg.Y <= 24:
		m.settingsField = 3
	case msg.Y >= 25 && msg.Y <= 28:
		m.settingsField = 4
	case msg.Y >= 29 && msg.Y <= 32:
		m.settingsField = 5
	case msg.Y >= 33 && msg.Y <= 36:
		m.settingsField = 6
	}
	return m, nil
}

func (m *tuiModel) clearTOTP() {
	m.totpID++
	vault.Zero(m.totpSecret)
	m.totpSecret = nil
	m.totpCode = ""
	m.totpSeconds = 0
}

func (m *tuiModel) clipboardStatus(message string) string {
	if message != "" {
		return message
	}
	return "Password copied; clipboard clears in 35s"
}

func (m *tuiModel) loginKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
		m.quitting = true
		return m, tea.Quit
	}
	if msg.Type == tea.KeyBackspace {
		if len(m.master) > 0 {
			_, n := utf8.DecodeLastRune(m.master)
			vault.Zero(m.master[len(m.master)-n:])
			m.master = m.master[:len(m.master)-n]
		}
		return m, nil
	}
	if msg.Type == tea.KeyEnter {
		if len(m.master) == 0 {
			m.status = "Master password cannot be empty"
			return m, nil
		}
		entries, err := vault.Load(m.path, m.master)
		if err != nil {
			// A rejected attempt must never leave the typed bytes behind:
			// stray hotkeys pressed on the locked screen silently accumulate
			// in this buffer, and without a reset every subsequent attempt
			// would re-submit "garbage + password" forever.
			vault.Zero(m.master)
			m.master = nil
			m.status = "Invalid master password"
			return m, nil
		}
		m.entries = entries
		m.mode = modeList
		m.locked = false
		m.lastActivity = time.Now()
		m.status = ""
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		for _, r := range msg.Runes {
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], r)
			m.master = append(m.master, buf[:n]...)
		}
	}
	return m, nil
}

func (m *tuiModel) addKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.returnToList()
		return m, nil
	}
	if msg.Type == tea.KeyTab || msg.Type == tea.KeyEnter {
		if m.editing && msg.Type == tea.KeyEnter && m.field == 1 {
			m.field = 2
			return m, nil
		}
		if m.field < 3 {
			m.field++
			return m, nil
		}
		if m.service == "" || m.login == "" || len(m.password) == 0 {
			m.status = "Service, login and password are required"
			return m, nil
		}
		for i, e := range m.entries {
			if e.Service == m.service && (!m.editing || i != m.editIndex) {
				m.status = "Service already exists"
				return m, nil
			}
		}
		var previous vault.Entry
		if m.editing {
			previous = vault.Entry{Service: m.entries[m.editIndex].Service, Login: m.entries[m.editIndex].Login, Password: append([]byte(nil), m.entries[m.editIndex].Password...), TotpSecret: append([]byte(nil), m.entries[m.editIndex].TotpSecret...)}
		}
		if m.editing {
			m.entries[m.editIndex].Service = m.service
			m.entries[m.editIndex].Login = m.login
			vault.Zero(m.entries[m.editIndex].Password)
			m.entries[m.editIndex].Password = append([]byte(nil), m.password...)
			vault.Zero(m.entries[m.editIndex].TotpSecret)
			m.entries[m.editIndex].TotpSecret = append([]byte(nil), m.totpSecret...)
		} else {
			m.entries = append(m.entries, vault.Entry{Service: m.service, Login: m.login, Password: append([]byte(nil), m.password...), TotpSecret: append([]byte(nil), m.totpSecret...)})
		}
		if err := vault.Save(m.path, m.master, m.entries); err != nil {
			m.status = err.Error()
			if m.editing {
				// Restore the in-memory record when the encrypted write fails.
				vault.Zero(m.entries[m.editIndex].Password)
				vault.Zero(m.entries[m.editIndex].TotpSecret)
				m.entries[m.editIndex] = previous
			} else {
				m.entries = m.entries[:len(m.entries)-1]
			}
			return m, nil
		}
		if m.editing {
			vault.Zero(previous.Password)
			vault.Zero(previous.TotpSecret)
		}
		vault.Zero(m.password)
		m.password = nil
		wasEditing := m.editing
		m.returnToList()
		m.status = map[bool]string{true: "Entry updated", false: "Entry added"}[wasEditing]
		m.selected = len(m.entries) - 1
		return m, nil
	}
	if msg.Type == tea.KeyBackspace {
		m.backspace()
		return m, nil
	}
	// Ctrl+G generates a password. A single 'g' hotkey used to shadow the
	// literal 'g' rune, making it impossible to type passwords containing
	// or starting with 'g'.
	if msg.Type == tea.KeyCtrlG && m.field == 2 {
		p, err := generatePassword(18)
		if err != nil {
			m.status = err.Error()
		} else {
			vault.Zero(m.password)
			m.password = p
			m.strength = passwordStrength(p)
			m.status = "Secure password generated"
		}
		return m, nil
	}
	// Ctrl+H checks the password against HIBP. A single 'h' hotkey used
	// to shadow the literal 'h' rune, making it impossible to type
	// passwords containing or starting with 'h'.
	if msg.Type == tea.KeyCtrlH && m.field == 2 && len(m.password) > 0 {
		p := append([]byte(nil), m.password...)
		return m, func() tea.Msg { defer vault.Zero(p); n, err := hibpCheck(p); return hibpMsg{found: n, err: err} }
	}
	if msg.Type != tea.KeyRunes {
		return m, nil
	}
	for _, r := range msg.Runes {
		if m.field == 0 {
			m.service += string(r)
		} else if m.field == 1 {
			m.login += string(r)
		} else if m.field == 2 {
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], r)
			m.password = append(m.password, buf[:n]...)
			m.strength = passwordStrength(m.password)
		} else if m.field == 3 {
			m.totpSecret = append(m.totpSecret, []byte(string(r))...)
		}
	}
	return m, nil
}

func (m *tuiModel) backspace() {
	if m.field < 2 {
		var s *string
		if m.field == 0 {
			s = &m.service
		} else {
			s = &m.login
		}
		if len(*s) > 0 {
			_, n := utf8.DecodeLastRuneInString(*s)
			*s = (*s)[:len(*s)-n]
		}
		return
	}
	if m.field == 2 && len(m.password) > 0 {
		_, n := utf8.DecodeLastRune(m.password)
		vault.Zero(m.password[len(m.password)-n:])
		m.password = m.password[:len(m.password)-n]
		m.strength = passwordStrength(m.password)
		return
	}
	if m.field == 3 && len(m.totpSecret) > 0 {
		m.totpSecret = trimLastRune(m.totpSecret)
	}
}

func (m *tuiModel) deleteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || (msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && hotkeyRune(msg.Runes[0]) == 'n') {
		m.mode = modeList
		return m, nil
	}
	if msg.Type != tea.KeyEnter && !(msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && (hotkeyRune(msg.Runes[0]) == 'y')) {
		return m, nil
	}
	if m.selected < 0 || m.selected >= len(m.entries) {
		m.selected = 0
		m.mode = modeList
		return m, nil
	}
	removed := m.entries[m.selected]
	kept := make([]vault.Entry, 0, len(m.entries)-1)
	kept = append(kept, m.entries[:m.selected]...)
	kept = append(kept, m.entries[m.selected+1:]...)
	m.entries = kept
	if err := vault.Save(m.path, m.master, m.entries); err != nil {
		m.entries = append(m.entries[:m.selected], append([]vault.Entry{removed}, m.entries[m.selected:]...)...)
		m.status = "Save failed: " + err.Error()
		m.mode = modeList
		return m, nil
	}
	vault.Zero(removed.Password)
	vault.Zero(removed.TotpSecret)
	if len(m.entries) == 0 {
		m.selected = 0
	} else if m.selected >= len(m.entries) {
		m.selected = len(m.entries) - 1
	}
	m.revealed = false
	m.mode, m.status = modeList, "Entry deleted"
	return m, nil
}

func (m *tuiModel) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.returnToList()
		m.status = ""
		return m, nil
	}
	if msg.Type == tea.KeyUp && m.settingsField > 0 {
		m.settingsField--
	}
	if msg.Type == tea.KeyDown && m.settingsField < maxSettingsIndex() {
		m.settingsField++
	}
	if msg.Type == tea.KeyRunes && m.settingsField == 2 {
		m.newMaster = append(m.newMaster, []byte(string(msg.Runes))...)
		return m, nil
	}
	if msg.Type == tea.KeyBackspace && m.settingsField == 2 && len(m.newMaster) > 0 {
		m.newMaster = trimLastRune(m.newMaster)
		return m, nil
	}
	if msg.Type != tea.KeyEnter {
		return m, nil
	}
	switch m.settingsField {
	case 0:
		switch m.lockDuration {
		case 0:
			m.lockDuration = time.Minute
		case time.Minute:
			m.lockDuration = 5 * time.Minute
		case 5 * time.Minute:
			m.lockDuration = 15 * time.Minute
		default:
			m.lockDuration = 0
		}
	case 1:
		switch m.clipboardDuration {
		case 15 * time.Second:
			m.clipboardDuration = 35 * time.Second
		case 35 * time.Second:
			m.clipboardDuration = 60 * time.Second
		default:
			m.clipboardDuration = 15 * time.Second
		}
	case 2:
		if len(m.newMaster) == 0 {
			m.status = "New master password cannot be empty"
			return m, nil
		}
		if err := vault.Save(m.path, m.newMaster, m.entries); err != nil {
			m.status = "Master password change failed: " + err.Error()
			return m, nil
		}
		vault.Zero(m.master)
		m.master = append([]byte(nil), m.newMaster...)
		vault.Zero(m.newMaster)
		m.newMaster = nil
		m.status = "Master password changed"
		m.settingsField = 0
	case 3:
		m.theme = nextTheme(m.theme)
		if err := saveTheme(m.theme); err != nil {
			m.status = "Theme save failed: " + err.Error()
		}
	case 4:
		m.transferPath = ""
		m.mode = modeImport
	case 5:
		name, err := encryptedBackup(m.path)
		if err != nil {
			m.status = "Backup failed: " + err.Error()
		} else {
			m.status = "Encrypted backup saved to " + name
		}
	case 6:
		m.mode = modeUnsafeExport
	}
	return m, nil
}

func (m *tuiModel) transferKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.returnToList()
		return m, nil
	}
	if m.mode == modeUnsafeExport {
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && (hotkeyRune(msg.Runes[0]) == 'y') {
			if err := plaintextCSV("kekkai-export.csv", m.entries); err != nil {
				m.status = "CSV export failed: " + err.Error()
			} else {
				m.status = "Plaintext CSV exported to kekkai-export.csv"
			}
			m.returnToList()
			return m, nil
		}
		return m, nil
	}
	if msg.Type == tea.KeyBackspace && len(m.transferPath) > 0 {
		_, n := utf8.DecodeLastRuneInString(m.transferPath)
		if n <= 0 || n > len(m.transferPath) {
			n = 1
		}
		m.transferPath = m.transferPath[:len(m.transferPath)-n]
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.transferPath += string(msg.Runes)
		return m, nil
	}
	if msg.Type == tea.KeyEnter {
		entries, count, err := importCSV(cleanTransferPath(m.transferPath), m.entries)
		if err != nil {
			m.status = "Import failed: " + err.Error()
			return m, nil
		}
		if err = vault.Save(m.path, m.master, entries); err != nil {
			clearEntries(entries)
			m.status = "Import save failed: " + err.Error()
			return m, nil
		}
		clearEntries(m.entries)
		m.entries = entries
		m.status = fmt.Sprintf("Imported %d entries. Please securely delete the unencrypted CSV!", count)
		m.returnToList()
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(fox.Render(banner))
	b.WriteString("\n")
	if m.mode == modeLogin {
		title := "UNLOCK VAULT"
		hint := "Enter unlocks · Esc quits"
		if m.locked {
			title = "VAULT LOCKED"
			hint = "Enter to unlock · Esc quits"
		}
		b.WriteString(panel.Render(title + "\n\nMaster password: " + strings.Repeat("•", utf8.RuneCount(m.master)) + "_\n\n" + muted.Render(hint)))
		if m.status != "" {
			b.WriteString("\n\n" + coral.Render("! "+m.status))
		}
		return b.String() + "\n"
	}
	if m.mode == modeAdd {
		return m.addView(&b)
	}
	if m.mode == modeSettings {
		return b.String() + m.settingsView() + "\n"
	}
	if m.mode == modeImport {
		return b.String() + panel.Render("IMPORT FROM CSV\n\nPath: "+m.transferPath+"_\n\nEnter import · Esc cancel") + "\n"
	}
	if m.mode == modeUnsafeExport {
		return b.String() + panel.Render("Are you sure? This will export unencrypted passwords!\n\nPress 'y' to confirm / Esc to cancel") + "\n"
	}
	if m.mode == modeConfirmDelete {
		name := ""
		if m.selected >= 0 && m.selected < len(m.entries) {
			name = m.entries[m.selected].Service
		}
		confirm := fmt.Sprintf("Delete %q?\n\n%s confirm   %s cancel", name, keyStyle.Render("y / Enter"), keyStyle.Render("n / Esc"))
		return b.String() + panel.Render(confirm) + "\n"
	}
	var list strings.Builder
	if len(m.entries) == 0 {
		list.WriteString(muted.Render("No entries yet. Press 'a' to add one."))
	}
	indices := m.visibleIndices()
	for display, index := range indices {
		e := m.entries[index]
		line := fmt.Sprintf("%s  %s", e.Service, muted.Render(e.Login))
		if index == m.selected || (m.selected >= len(m.entries) && display == 0) {
			line = selected.Render("› " + e.Service + "  " + e.Login)
		}
		list.WriteString(line + "\n")
	}
	b.WriteString(panel.Render(list.String()))
	if m.searching {
		b.WriteString("\n" + soft.Render("SEARCH  ") + fox.Render(m.search+"_") + muted.Render("  type to filter · ↑/↓ navigate · Enter done · Esc clear") + "\n")
	} else if m.search != "" {
		b.WriteString("\n" + soft.Render("FILTER  ") + fox.Render(m.search) + muted.Render("  / new search · Esc clear") + "\n")
	}
	if len(m.entries) > 0 && m.revealed && m.selected >= 0 && m.selected < len(m.entries) {
		password := coral.Render(string(m.entries[m.selected].Password))
		b.WriteString("\n" + panel.Render("PASSWORD  "+password))
	}
	if m.totpCode != "" {
		b.WriteString("\n" + panel.Render(fmt.Sprintf("TOTP  %s   refreshes in %ds", m.totpCode, m.totpSeconds)))
	}
	b.WriteString("\n\n" + keyStyle.Render("↑/↓") + muted.Render(" navigate · ") + keyStyle.Render("Enter") + muted.Render(" reveal · ") + keyStyle.Render("c") + muted.Render(" copy pass · ") + keyStyle.Render("u") + muted.Render(" copy user · ") + keyStyle.Render("t") + muted.Render(" copy TOTP · ") + keyStyle.Render("a") + muted.Render(" add · ") + keyStyle.Render("e") + muted.Render(" edit · ") + keyStyle.Render("d") + muted.Render(" delete · ") + keyStyle.Render("/") + muted.Render(" search · ") + keyStyle.Render("s") + muted.Render(" settings · ") + keyStyle.Render("q") + muted.Render(" quit\n"))
	if m.status != "" {
		b.WriteString("\n" + soft.Render("• "+m.status) + "\n")
	}
	return b.String()
}

func (m *tuiModel) settingsView() string {
	rows := []string{
		fmt.Sprintf("Lock timeout: %s", durationLabel(m.lockDuration)),
		fmt.Sprintf("Clipboard clear: %s", durationLabel(m.clipboardDuration)),
		"New master: " + strings.Repeat("•", utf8.RuneCount(m.newMaster)),
		"Theme: " + m.theme.Name,
		"Import from CSV",
		"Export Backup (.enc)",
		"Export to CSV (Unsafe)",
	}
	var content strings.Builder
	content.WriteString("SETTINGS\n\n")
	for i, row := range rows {
		if i == m.settingsField {
			content.WriteString(fox.Render("> " + row))
		} else {
			content.WriteString(muted.Render("  " + row))
		}
		content.WriteByte('\n')
	}
	content.WriteString("\n" + soft.Render("↑/↓ or mouse wheel select · Enter change · type new master · Esc back"))
	return panel.Render(content.String())
}

func (m *tuiModel) addView(b *strings.Builder) string {
	b.WriteString(panel.Render("ADD NEW ENTRY\n" + muted.Render("Tab/Enter advances · Esc cancels") + "\n\n"))
	fmt.Fprintf(b, "Service:  %s", m.service)
	if m.field == 0 {
		b.WriteString("_")
	}
	fmt.Fprintf(b, "\nLogin:    %s", m.login)
	if m.field == 1 {
		b.WriteString("_")
	}
	b.WriteString("\nPassword: ")
	if m.field == 2 {
		b.WriteString(strings.Repeat("*", len(m.password)))
		b.WriteString("_")
	} else if len(m.password) > 0 {
		b.WriteString(strings.Repeat("*", len(m.password)))
	}
	if m.strength != "" {
		b.WriteString("\nStrength: " + strengthStyle(m.strength))
	}
	b.WriteString("\nTOTP secret: ")
	if m.field == 3 {
		b.WriteString(string(m.totpSecret) + "_")
	} else {
		b.WriteString(string(m.totpSecret))
	}
	b.WriteString("\n" + muted.Render("Ctrl+G generate password · Ctrl+H check HIBP · type TOTP secret in next field") + "\n\n" + soft.Render(m.status) + "\n")
	return b.String()
}

func durationLabel(d time.Duration) string {
	if d == 0 {
		return "Off"
	}
	return d.String()
}

func (m *tuiModel) visibleIndices() []int {
	out := make([]int, 0, len(m.entries))
	for i, e := range m.entries {
		if m.search == "" || fuzzyMatch(m.search, e.Service) {
			out = append(out, i)
		}
	}
	return out
}
func strengthStyle(s string) string {
	switch s {
	case "STRONG":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#61D095")).Bold(true).Render("■■■■ STRONG")
	case "MEDIUM":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F5B84B")).Bold(true).Render("■■■□ MEDIUM")
	default:
		return coral.Render("■■□□ WEAK")
	}
}
