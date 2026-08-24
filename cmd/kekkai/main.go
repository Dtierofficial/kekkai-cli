package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
