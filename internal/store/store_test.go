package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// newTestStore builds a store backed by memory and a temporary directory, so
// no test touches the real keychain. The keychain and the disk backend are
// returned too, for the tests that need to look behind the Store.
func newTestStore(t *testing.T) (*Store, *memoryKeychain, disk) {
	t.Helper()
	dir := t.TempDir()
	kc := newMemoryKeychain()
	s, err := newWithKeychain(dir, kc)
	if err != nil {
		t.Fatalf("newWithKeychain: %v", err)
	}
	return s, kc, disk{dir: dir, keys: kc}
}

// largeValue is bigger than the keychain ceiling, so it takes the encrypted
// file path.
func largeValue() []byte { return []byte(strings.Repeat("S", 5000)) }

func TestSetGetRoundTrip(t *testing.T) {
	cases := map[string][]byte{
		"small": []byte("a-token-value"),
		"large": largeValue(),
		"empty": {},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newTestStore(t)
			if err := s.Set("key", want); err != nil {
				t.Fatalf("Set: %v", err)
			}
			got, err := s.Get("key")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("got %d bytes, want %d", len(got), len(want))
			}
		})
	}
}

// Replacing a value must not leave the previous copy readable on the other
// storage path.
func TestSetReplacesAcrossPaths(t *testing.T) {
	t.Run("large to small", func(t *testing.T) {
		s, _, d := newTestStore(t)
		if err := s.Set("key", largeValue()); err != nil {
			t.Fatalf("Set large: %v", err)
		}
		if err := s.Set("key", []byte("small")); err != nil {
			t.Fatalf("Set small: %v", err)
		}

		got, err := s.Get("key")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if string(got) != "small" {
			t.Fatalf("got %q, want small", got)
		}
		if _, err := os.Stat(d.path("key")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("the encrypted file was left behind")
		}
	})

	t.Run("small to large", func(t *testing.T) {
		s, _, _ := newTestStore(t)
		if err := s.Set("key", []byte("small")); err != nil {
			t.Fatalf("Set small: %v", err)
		}
		want := largeValue()
		if err := s.Set("key", want); err != nil {
			t.Fatalf("Set large: %v", err)
		}

		got, err := s.Get("key")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("got %d bytes, want %d", len(got), len(want))
		}
	})
}

func TestGetMissingKey(t *testing.T) {
	s, _, _ := newTestStore(t)
	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("errors.Is(err, ErrNotFound) = false, err = %v", err)
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	s, _, _ := newTestStore(t)
	if err := s.Set("key", largeValue()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	for i := range 2 {
		if err := s.Remove("key"); err != nil {
			t.Fatalf("Remove #%d: %v", i+1, err)
		}
	}
	if _, err := s.Get("key"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("value survived Remove: %v", err)
	}
	keys, err := s.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("Keys = %v, want empty", keys)
	}
}

func TestClearRemovesEverything(t *testing.T) {
	s, _, d := newTestStore(t)
	if err := s.Set("token", []byte("t")); err != nil {
		t.Fatalf("Set token: %v", err)
	}
	if err := s.Set("keystore", largeValue()); err != nil {
		t.Fatalf("Set keystore: %v", err)
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	for _, key := range []string{"token", "keystore"} {
		if _, err := s.Get(key); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s survived Clear: %v", key, err)
		}
	}
	keys, err := s.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("Keys = %v, want empty", keys)
	}

	// No encrypted files should be left in the state directory.
	entries, err := filepath.Glob(filepath.Join(d.dir, "*.enc"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("encrypted files left behind: %v", entries)
	}
}

func TestKeysTracksStoredValues(t *testing.T) {
	s, _, _ := newTestStore(t)
	if err := s.Set("a", []byte("1")); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	// Setting the same key twice must not duplicate it in the index.
	if err := s.Set("a", []byte("2")); err != nil {
		t.Fatalf("Set a again: %v", err)
	}
	if err := s.Set("b", []byte("3")); err != nil {
		t.Fatalf("Set b: %v", err)
	}

	keys, err := s.Keys()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("Keys = %v, want 2 entries", keys)
	}
}

func TestDiskFileIsEncryptedAndPrivate(t *testing.T) {
	s, _, d := newTestStore(t)
	secret := []byte(strings.Repeat("PRIVATE-KEY-MATERIAL", 250))
	if err := s.Set("keystore", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}

	path := d.path("keystore")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != filePerm {
		t.Errorf("permissions = %v, want %v", perm, filePerm)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(onDisk, []byte("PRIVATE-KEY-MATERIAL")) {
		t.Fatal("the value was written to disk in the clear")
	}
}

// Without its key in the keychain, a value on disk must not be readable.
func TestDiskValueNeedsItsKey(t *testing.T) {
	s, kc, _ := newTestStore(t)
	if err := s.Set("keystore", largeValue()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := kc.Remove("keystore" + dekSuffix); err != nil {
		t.Fatalf("Remove key: %v", err)
	}
	if _, err := s.Get("keystore"); err == nil {
		t.Fatal("the value was readable without its encryption key")
	}
}

func TestJSONHelpers(t *testing.T) {
	type keystore struct {
		UserEmail string `json:"userEmail"`
		Private   string `json:"privateKeyEncrypted"`
	}

	s, _, _ := newTestStore(t)
	want := keystore{UserEmail: "user@inxt.com", Private: strings.Repeat("K", 4000)}
	if err := SetJSON(s, "keystore", want); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	var got keystore
	if err := GetJSON(s, "keystore", &got); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip mismatch")
	}
}

// Run with -race to check the mutex actually guards concurrent access.
func TestConcurrentAccess(t *testing.T) {
	s, _, _ := newTestStore(t)
	if err := s.Set("key", []byte("initial")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.Set("key", []byte{byte(i)}); err != nil {
				t.Errorf("Set: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Get("key"); err != nil && !errors.Is(err, ErrNotFound) {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()
}
