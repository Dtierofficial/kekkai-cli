package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.kek")
	master := []byte("correct horse battery staple")
	entries := []Entry{{Service: "example", Login: "alice", Password: []byte("s3cret")}}
	if err := New(path, master); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, master, entries); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, master)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for i := range got {
			Zero(got[i].Password)
		}
	}()
	if len(got) != 1 || got[0].Service != "example" || got[0].Login != "alice" || string(got[0].Password) != "s3cret" {
		t.Fatalf("unexpected entries: %#v", got)
	}
}

func TestTamperingAndWrongPasswordFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.kek")
	master := []byte("master")
	if err := Save(path, master, []Entry{{Service: "x", Password: []byte("y")}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 1
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, master); err == nil {
		t.Fatal("tampered vault was accepted")
	}
	if err := Save(path, master, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, []byte("wrong")); err == nil {
		t.Fatal("wrong password was accepted")
	}
}
