package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"kekkai/vault"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kekkai:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return interactive()
	}
	path, err := vaultPath()
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return initVault(path)
	case "add":
		if len(args) != 3 {
			return errors.New("usage: kekkai add <service> <login>")
		}
		return add(path, args[1], args[2])
	case "get":
		if len(args) != 2 {
			return errors.New("usage: kekkai get <service>")
		}
		return get(path, args[1])
	case "list":
		if len(args) != 1 {
			return errors.New("usage: kekkai list")
		}
		return list(path)
	case "delete":
		if len(args) != 2 {
			return errors.New("usage: kekkai delete <service>")
		}
		return remove(path, args[1])
	case "import":
		if len(args) != 2 {
			return errors.New("usage: kekkai import <file.csv>")
		}
		return importCmd(path, args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func vaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "kekkai", "vault.kek"), nil
}

func readSecret(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return b, err
}

func initVault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return errors.New("vault already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	p, err := readSecret("Master password: ")
	if err != nil {
		return err
	}
	defer vault.Zero(p)
	confirm, err := readSecret("Confirm master password: ")
	if err != nil {
		return err
	}
	defer vault.Zero(confirm)
	if len(p) == 0 {
		return errors.New("master password cannot be empty")
	}
	if !equal(p, confirm) {
		return errors.New("passwords do not match")
	}
	return vault.New(path, p)
}

func load(path string) ([]vault.Entry, []byte, error) {
	p, err := readSecret("Master password: ")
	if err != nil {
		return nil, nil, err
	}
	entries, err := vault.Load(path, p)
	if err != nil {
		vault.Zero(p)
		return nil, nil, err
	}
	return entries, p, nil
}

func add(path, service, login string) error {
	entries, master, err := load(path)
	if err != nil {
		return err
	}
	defer vault.Zero(master)
	pass, err := readSecret("Entry password: ")
	if err != nil {
		return err
	}
	defer vault.Zero(pass)
	for _, e := range entries {
		// Same pair as the TUI: one service may hold several logins.
		if e.Service == service && e.Login == login {
			return errors.New("entry already exists")
		}
	}
	entries = append(entries, vault.Entry{Service: service, Login: login, Password: pass})
	err = vault.Save(path, master, entries)
	for i := range entries {
		vault.Zero(entries[i].Password)
		vault.Zero(entries[i].TotpSecret)
	}
	return err
}

// importCmd is the CLI entry point for CSV import. The master password is
// always requested with hidden input - never as an argument, where it would
// land in shell history and the process list.
func importCmd(path, file string) error {
	stored, master, err := load(path)
	if err != nil {
		return err
	}
	return runImport(path, file, stored, master, confirmYesNo)
}

// runImport parses the CSV, previews the new entries and only after an
// explicit confirmation rewrites the vault. Any parse error leaves the
// vault untouched.
func runImport(path, file string, stored []vault.Entry, master []byte, confirm func(string) bool) error {
	defer vault.Zero(master)
	defer clearEntries(stored)

	merged, _, err := importCSV(cleanTransferPath(file), stored)
	if err != nil {
		clearEntries(merged[len(stored):])
		return err
	}
	added := merged[len(stored):]
	if len(added) == 0 {
		return errors.New("nothing to import: file is empty, or every row duplicates an existing entry")
	}

	fmt.Fprintf(os.Stderr, "Will add %d entries:\n", len(added))
	for _, e := range added {
		suffix := ""
		if len(e.TotpSecret) > 0 {
			suffix = "  (TOTP)"
		}
		// Services and logins only: passwords never touch the terminal.
		fmt.Fprintf(os.Stderr, "  %s  %s%s\n", e.Service, e.Login, suffix)
	}

	if !confirm(fmt.Sprintf("Import %d entries into the vault? [y/N]: ", len(added))) {
		clearEntries(added)
		return errors.New("import cancelled; vault unchanged")
	}
	if err := vault.Save(path, master, merged); err != nil {
		clearEntries(added)
		return err
	}
	clearEntries(merged)
	fmt.Fprintf(os.Stderr, "Imported %d entries. Delete the plaintext CSV now: %s\n", len(added), file)
	return nil
}

// confirmYesNo reads a y/n answer from stdin. The default (anything but
// y/yes) is "no".
func confirmYesNo(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func get(path, service string) error {
	entries, master, err := load(path)
	if err != nil {
		return err
	}
	defer vault.Zero(master)
	defer clearEntries(entries)
	for _, e := range entries {
		if e.Service == service {
			fmt.Fprintf(os.Stdout, "Login: %s\nPassword: ", e.Login)
			_, _ = os.Stdout.Write(e.Password)
			_, _ = io.WriteString(os.Stdout, "\n")
			return nil
		}
	}
	return errors.New("service not found")
}

func list(path string) error {
	entries, master, err := load(path)
	if err != nil {
		return err
	}
	defer vault.Zero(master)
	defer clearEntries(entries)
	for _, e := range entries {
		fmt.Fprintf(os.Stdout, "%s (%s)\n", e.Service, e.Login)
	}
	return nil
}

func remove(path, service string) error {
	entries, master, err := load(path)
	if err != nil {
		return err
	}
	defer vault.Zero(master)
	found := false
	kept := entries[:0]
	for _, e := range entries {
		if e.Service == service {
			found = true
			vault.Zero(e.Password)
		} else {
			kept = append(kept, e)
		}
	}
	if !found {
		clearEntries(entries)
		return errors.New("service not found")
	}
	err = vault.Save(path, master, kept)
	clearEntries(entries)
	return err
}

func clearEntries(entries []vault.Entry) {
	for i := range entries {
		vault.Zero(entries[i].Password)
		vault.Zero(entries[i].TotpSecret)
	}
}
func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
