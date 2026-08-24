package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kekkai/vault"
)

// maxImportRows caps CSV imports so a huge or malicious file cannot exhaust
// memory before the vault's own entry limit is enforced at save time.
const maxImportRows = 10000

func importCSV(path string, entries []vault.Entry) ([]vault.Entry, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return entries, 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return entries, 0, err
	}
	cols := make(map[string]int)
	for i, h := range header {
		cols[strings.ToLower(strings.TrimSpace(h))] = i
	}
	// "service" keeps kekkai's own exports importable, "url"/"name" accept
	// Bitwarden- and Chrome-style CSV files.
	serviceCol, okURL := cols["url"]
	if !okURL {
		serviceCol, okURL = cols["name"]
	}
	if !okURL {
		serviceCol, okURL = cols["service"]
	}
	if !okURL {
		return entries, 0, errors.New("CSV must contain url, name or service column")
	}
	loginCol, passCol := cols["username"], cols["password"]
	if _, ok := cols["username"]; !ok {
		return entries, 0, errors.New("CSV must contain username column")
	}
	if _, ok := cols["password"]; !ok {
		return entries, 0, errors.New("CSV must contain password column")
	}
	totpCol, hasTotp := cols["totp"]
	count := 0
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return entries, count, e
		}
		get := func(i int) string {
			if i < 0 || i >= len(row) {
				return ""
			}
			return row[i]
		}
		service, login, pass := get(serviceCol), get(loginCol), get(passCol)
		if service == "" || pass == "" {
			continue
		}
		duplicate := false
		for _, old := range entries {
			if old.Service == service && old.Login == login {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		entry := vault.Entry{Service: service, Login: login, Password: []byte(pass)}
		if hasTotp {
			entry.TotpSecret = []byte(get(totpCol))
		}
		entries = append(entries, entry)
		count++
		if len(entries) > maxImportRows {
			return entries, count, errors.New("too many entries in CSV")
		}
	}
	return entries, count, nil
}

func encryptedBackup(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("kekkai-backup-%s.enc", time.Now().Format("20060102-150405"))
	if err = os.WriteFile(name, b, 0600); err != nil {
		return "", err
	}
	return name, nil
}

func plaintextCSV(path string, entries []vault.Entry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err = w.Write([]string{"service", "username", "password", "totp"}); err != nil {
		return err
	}
	for _, e := range entries {
		if err = w.Write([]string{e.Service, e.Login, string(e.Password), string(e.TotpSecret)}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// cleanTransferPath normalizes user input for os.Open. Explorer's
// "Copy as path" wraps the whole path in quotes, so stray quote characters
// must be stripped before use.
func cleanTransferPath(path string) string {
	return filepath.Clean(strings.TrimSpace(strings.Trim(strings.TrimSpace(path), `"'`)))
}
