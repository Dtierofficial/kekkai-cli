package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

const idleTimeout = 5 * time.Minute

type tickMsg time.Time
type hibpMsg struct {
	found int
	err   error
}
type totpTickMsg struct{ id uint64 }

func totpCode(secret []byte, now time.Time) (string, int, error) {
	clean := normalizeTOTPSecret(string(secret))
	if clean == "" {
		return "", 0, errors.New("no TOTP secret configured")
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(clean, "="))
	if err != nil {
		return "", 0, fmt.Errorf("invalid TOTP secret: %w", err)
	}
	step := now.Unix() / 30
	var counter [8]byte
	for i := 7; i >= 0; i-- {
		counter[i] = byte(step)
		step >>= 8
	}
	h := hmac.New(sha1.New, key)
	_, _ = h.Write(counter[:])
	sum := h.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000), 30 - int(now.Unix()%30), nil
}

func normalizeTOTPSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if parsed, err := url.Parse(raw); err == nil && strings.EqualFold(parsed.Scheme, "otpauth") {
		if value := parsed.Query().Get("secret"); value != "" {
			raw = value
		}
	}
	var b strings.Builder
	for _, r := range raw {
		if r == '-' || r == '=' {
			continue
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '2' && r <= '7' {
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(strings.TrimRight(b.String(), "="))
}

type clipboardCopiedMsg struct {
	id      uint64
	err     error
	message string
}
type clipboardClearDueMsg struct{ id uint64 }
type clipboardClearedMsg struct {
	id  uint64
	err error
}
type clipboardHideMsg struct{ id uint64 }

func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func passwordStrength(p []byte) string {
	score := 0
	if len(p) >= 12 {
		score++
	}
	if len(p) >= 16 {
		score++
	}
	var lower, upper, digit, special bool
	for _, c := range p {
		switch {
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= 'A' && c <= 'Z':
			upper = true
		case c >= '0' && c <= '9':
			digit = true
		default:
			special = true
		}
	}
	for _, yes := range []bool{lower, upper, digit, special} {
		if yes {
			score++
		}
	}
	if score >= 5 {
		return "STRONG"
	}
	if score >= 3 {
		return "MEDIUM"
	}
	return "WEAK"
}

func generatePassword(length int) ([]byte, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+"
	if length < 16 {
		length = 16
	}
	out := make([]byte, length)
	random := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return nil, err
	}
	defer zeroBytes(random)
	for i, b := range random {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return out, nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func fuzzyMatch(query, value string) bool {
	query, value = strings.ToLower(query), strings.ToLower(value)
	pos := 0
	for _, q := range query {
		found := false
		for pos < len(value) {
			r, size := utf8.DecodeRuneInString(value[pos:])
			if r == q {
				found = true
				pos += size
				break
			}
			pos += size
		}
		if !found {
			return false
		}
	}
	return true
}

func copyClipboard(value []byte) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "$input | Set-Clipboard")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	if _, err = in.Write(value); err != nil {
		_ = in.Close()
		return err
	}
	if err = in.Close(); err != nil {
		return err
	}
	return cmd.Wait()
}

func clearClipboard() error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard -Value \"\"").Run()
	case "darwin":
		return exec.Command("pbcopy").Run()
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command("sh", "-c", "printf '' | wl-copy").Run()
		}
		return exec.Command("sh", "-c", "printf '' | xclip -selection clipboard").Run()
	}
}

func copyClipboardCmd(id uint64, value []byte, message string) tea.Cmd {
	copied := append([]byte(nil), value...)
	return func() tea.Msg {
		defer zeroBytes(copied)
		if err := copyClipboard(copied); err != nil {
			return clipboardCopiedMsg{id: id, err: err, message: message}
		}
		return clipboardCopiedMsg{id: id, message: message}
	}
}

func clearClipboardCmd(id uint64) tea.Cmd {
	return func() tea.Msg { return clipboardClearedMsg{id: id, err: clearClipboard()} }
}

func hibpCheck(password []byte) (int, error) {
	h := sha1.Sum(password)
	encoded := strings.ToUpper(hex.EncodeToString(h[:]))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.pwnedpasswords.com/range/" + encoded[:5])
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HIBP returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	suffix := encoded[5:]
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], suffix) {
			n, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			return n, nil
		}
	}
	return 0, nil
}

var errClipboardUnavailable = errors.New("clipboard command is unavailable")
