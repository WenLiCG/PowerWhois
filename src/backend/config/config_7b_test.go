package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The production seams deliberately remain unimplemented in the RED slice:
// writeConfigChown func(string, int, int) error; writeConfigSync func(*os.File) error.
// All behavior is exercised through WriteConfig, never by calling the seams.
func TestPW7bWriteOwnershipSyncAndAtomicDirectory(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "chown-error"
		}
		t.Run(name, func(t *testing.T) {
			path := pw6ConfigPath(t)
			original := []byte("# pw7b original\n")
			if err := os.WriteFile(path, original, 0640); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			owner := before.Sys().(*syscall.Stat_t)
			other := t.TempDir()
			t.Setenv("TMPDIR", other)
			oldChown, oldSync := writeConfigChown, writeConfigSync
			t.Cleanup(func() { writeConfigChown, writeConfigSync = oldChown, oldSync })
			chowns, syncs := 0, 0
			checkTemporary := func(temp string) {
				t.Helper()
				if temp == path {
					t.Error("hook operated on destination, want temporary file")
				}
				if filepath.Dir(temp) != filepath.Dir(path) {
					t.Error("temporary file must be in config directory")
				}
				if _, err := os.Stat(temp); err != nil {
					t.Errorf("temporary file missing: %v", err)
				}
				body, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(body, original) {
					t.Error("destination replaced before hook")
				}
			}
			injected := errors.New("pw7b injected chown failure")
			writeConfigChown = func(temp string, uid, gid int) error {
				chowns++
				checkTemporary(temp)
				if uid != int(owner.Uid) || gid != int(owner.Gid) {
					t.Errorf("chown uid/gid=%d:%d, want original owner", uid, gid)
				}
				if fail {
					return injected
				}
				return os.Chown(temp, uid, gid)
			}
			writeConfigSync = func(f *os.File) error { syncs++; checkTemporary(f.Name()); return f.Sync() }
			err = WriteConfig(Config{LogLevel: "Debug"})
			if chowns != 1 {
				t.Errorf("chown calls=%d, want 1", chowns)
			}
			if fail {
				if !errors.Is(err, injected) {
					t.Errorf("error=%v, want injected chown error", err)
				}
				body, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(body, original) {
					t.Error("chown failure changed original content")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if syncs != 1 {
					t.Errorf("Sync calls=%d, want 1 before rename", syncs)
				}
			}
			after, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			got := after.Sys().(*syscall.Stat_t)
			if got.Uid != owner.Uid || got.Gid != owner.Gid {
				t.Error("destination owner changed")
			}
			if fail {
				if after.Mode().Perm() != 0640 {
					t.Error("failure changed original mode, want 0640")
				}
			} else if after.Mode().Perm() != 0600 {
				t.Error("successful save mode, want 0600")
			}
			pw6NoTemporaryFiles(t, path)
			entries, err := os.ReadDir(other)
			if err != nil || len(entries) != 0 {
				t.Error("system temporary directory must remain empty")
			}
		})
	}
}

func TestPW7bWriteRealOwnerRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to create a 996:994 fixture; QA must run as root")
	}
	path := pw6ConfigPath(t)
	if err := os.WriteFile(path, []byte("# old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 996, 994); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	if st.Uid != 996 || st.Gid != 994 || info.Mode().Perm() != 0600 {
		t.Errorf("owner/mode=%d:%d %04o, want 996:994 0600", st.Uid, st.Gid, info.Mode().Perm())
	}
}

func TestPW7bWriteFirstFileProcessOwner(t *testing.T) {
	path := pw6ConfigPath(t)
	if err := WriteConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	if int(st.Uid) != os.Geteuid() || int(st.Gid) != os.Getegid() || info.Mode().Perm() != 0600 {
		t.Errorf("first config owner/mode=%d:%d %04o, want process euid/egid and 0600", st.Uid, st.Gid, info.Mode().Perm())
	}
	pw6NoTemporaryFiles(t, path)
}
