package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

var magic = [4]byte{'K', 'K', 'V', '1'}

const (
	version      byte = 2
	saltSize          = 16
	nonceSize         = 12
	keySize           = 32
	maxEntries        = 10000
	maxFieldSize      = 1 << 20
)

type Entry struct {
	Service    string
	Login      string
	Password   []byte
	TotpSecret []byte
}

func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func New(path string, master []byte) error {
	if len(master) == 0 {
		return errors.New("master password cannot be empty")
	}
	return save(path, master, nil)
}

func Load(path string, master []byte) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	defer Zero(b)
	if len(b) < 4+1+saltSize+nonceSize {
		return nil, errors.New("vault is truncated")
	}
	if string(b[:4]) != string(magic[:]) || (b[4] != 1 && b[4] != version) {
		return nil, errors.New("unsupported vault format")
	}
	salt := b[5 : 5+saltSize]
	nonce := b[5+saltSize : 5+saltSize+nonceSize]
	key := argon2.IDKey(master, salt, 3, 64*1024, 4, keySize)
	defer Zero(key)
	bl, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithNonceSize(bl, nonceSize)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, b[5+saltSize+nonceSize:], nil)
	if err != nil {
		return nil, errors.New("invalid master password or corrupted vault")
	}
	defer Zero(plain)
	return decodeEntries(plain, b[4] == version)
}

func Save(path string, master []byte, entries []Entry) error { return save(path, master, entries) }

func save(path string, master []byte, entries []Entry) error {
	if len(master) == 0 {
		return errors.New("master password cannot be empty")
	}
	plain, err := encodeEntries(entries)
	if err != nil {
		return err
	}
	defer Zero(plain)
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	defer Zero(salt)
	defer Zero(nonce)
	if _, err = io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	key := argon2.IDKey(master, salt, 3, 64*1024, 4, keySize)
	defer Zero(key)
	bl, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCMWithNonceSize(bl, nonceSize)
	if err != nil {
		return err
	}
	ciphertext := aead.Seal(nil, nonce, plain, nil)
	defer Zero(ciphertext)
	file := make([]byte, 0, 5+saltSize+nonceSize+len(ciphertext))
	file = append(file, magic[:]...)
	file = append(file, version)
	file = append(file, salt...)
	file = append(file, nonce...)
	file = append(file, ciphertext...)
	defer Zero(file)
	return atomicWrite(path, file)
}

func encodeEntries(entries []Entry) ([]byte, error) {
	if len(entries) > maxEntries {
		return nil, errors.New("too many entries")
	}
	size := 4
	for _, e := range entries {
		if len(e.Service) == 0 || len(e.Service) > maxFieldSize || len(e.Login) > maxFieldSize || len(e.Password) > maxFieldSize || len(e.TotpSecret) > maxFieldSize {
			return nil, errors.New("entry field is invalid or too large")
		}
		size += 16 + len(e.Service) + len(e.Login) + len(e.Password) + len(e.TotpSecret)
	}
	b := make([]byte, size)
	binary.BigEndian.PutUint32(b, uint32(len(entries)))
	off := 4
	for _, e := range entries {
		binary.BigEndian.PutUint32(b[off:], uint32(len(e.Service)))
		off += 4
		copy(b[off:], e.Service)
		off += len(e.Service)
		binary.BigEndian.PutUint32(b[off:], uint32(len(e.Login)))
		off += 4
		copy(b[off:], e.Login)
		off += len(e.Login)
		binary.BigEndian.PutUint32(b[off:], uint32(len(e.Password)))
		off += 4
		copy(b[off:], e.Password)
		off += len(e.Password)
		binary.BigEndian.PutUint32(b[off:], uint32(len(e.TotpSecret)))
		off += 4
		copy(b[off:], e.TotpSecret)
		off += len(e.TotpSecret)
	}
	return b, nil
}

func decodeEntries(b []byte, hasTOTP bool) (entries []Entry, err error) {
	if len(b) < 4 {
		return nil, errors.New("invalid vault payload")
	}
	n := binary.BigEndian.Uint32(b)
	if n > maxEntries {
		return nil, errors.New("invalid entry count")
	}
	off := 4
	entries = make([]Entry, 0, n)
	defer func() {
		if err != nil {
			for i := range entries {
				Zero(entries[i].Password)
				Zero(entries[i].TotpSecret)
			}
		}
	}()
	for i := uint32(0); i < n; i++ {
		read := func() ([]byte, error) {
			if len(b)-off < 4 {
				return nil, errors.New("invalid vault payload")
			}
			l := binary.BigEndian.Uint32(b[off:])
			off += 4
			if l > maxFieldSize || uint64(l) > uint64(len(b)-off) {
				return nil, errors.New("invalid vault payload")
			}
			v := append([]byte(nil), b[off:off+int(l)]...)
			off += int(l)
			return v, nil
		}
		s, err := read()
		if err != nil {
			return nil, err
		}
		l, err := read()
		if err != nil {
			Zero(s)
			return nil, err
		}
		p, err := read()
		if err != nil {
			Zero(s)
			Zero(l)
			Zero(p)
			return nil, err
		}
		var totp []byte
		if hasTOTP {
			totp, err = read()
			if err != nil {
				Zero(s)
				Zero(l)
				Zero(p)
				return nil, err
			}
		}
		entries = append(entries, Entry{Service: string(s), Login: string(l), Password: p, TotpSecret: totp})
		Zero(s)
		Zero(l)
	}
	if off != len(b) {
		return nil, errors.New("trailing data in vault payload")
	}
	return entries, nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".kekkai-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = atomicReplace(tmpName, path); err != nil {
		return fmt.Errorf("replace vault: %w", err)
	}
	return nil
}
