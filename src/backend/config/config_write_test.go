package config

import (
	"bytes"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
	"typonamer/log"
)

// These tests change process-wide state and must not run in parallel.
func pw6ConfigPath(t *testing.T) string {
	t.Helper()
	old := configFile
	configFile = filepath.Join(t.TempDir(), "config.yaml")
	t.Cleanup(func() { configFile = old })
	return configFile
}

func pw6NoTemporaryFiles(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Errorf("configuration directory has unexpected temporary files: %v", entries)
	}
}

func TestPW6WriteFailurePreservesOriginal(t *testing.T) {
	path := pw6ConfigPath(t)
	original := []byte("LogLevel: Info\nSocketProxyPassword: pw6-test-only-old-secret\n# " + strings.Repeat("original-padding-", 100) + "\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_FSIZE, &saved); err != nil {
		t.Fatal(err)
	}
	ignored := signal.Ignored(unix.SIGXFSZ)
	signal.Ignore(unix.SIGXFSZ)
	defer func() {
		if ignored {
			signal.Ignore(unix.SIGXFSZ)
		} else {
			signal.Reset(unix.SIGXFSZ)
		}
	}()
	defer func() {
		if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &saved); err != nil {
			t.Errorf("restore RLIMIT_FSIZE: %v", err)
		}
	}()
	limited := saved
	limited.Cur = 128
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limited); err != nil {
		t.Fatalf("cannot inject write failure with RLIMIT_FSIZE: %v", err)
	}
	writeErr := WriteConfig(Config{SocketProxyPassword: "pw6-test-only-new-secret-" + strings.Repeat("x", 256)})
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &saved); err != nil {
		t.Fatalf("restore RLIMIT_FSIZE: %v", err)
	}
	if writeErr == nil {
		t.Error("WriteConfig succeeded despite injected file-size limit; want error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("failed write changed original config bytes: got %d bytes, want original %d bytes", len(after), len(original))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("failed write changed original permissions: got %04o, want %04o", info.Mode().Perm(), before.Mode().Perm())
	}
	pw6NoTemporaryFiles(t, path)
}

func TestPW6WriteSuccessMode0600(t *testing.T) {
	for _, existing := range []bool{true, false} {
		name := "new-file"
		if existing {
			name = "existing-0644-L1"
		}
		t.Run(name, func(t *testing.T) {
			path := pw6ConfigPath(t)
			oldMask := unix.Umask(0)
			defer unix.Umask(oldMask)
			if existing {
				if err := os.WriteFile(path, []byte("SocketProxyPassword: pw6-test-only-replaced-secret\n"), 0644); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0644 {
					t.Fatalf("fixture mode=%04o, want 0644", info.Mode().Perm())
				}
			}
			const password = "pw6-test-only-success-secret"
			if err := WriteConfig(Config{SocketProxyPassword: password}); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(body, []byte("SocketProxyPassword: "+password)) || bytes.Contains(body, []byte("pw6-test-only-replaced-secret")) {
				t.Error("successful write did not install new configuration")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Errorf("mode=%04o, want 0600 under umask 000", info.Mode().Perm())
			}
			pw6NoTemporaryFiles(t, path)
		})
	}
}

func TestPW6DebugLogOmitsConfigYAML(t *testing.T) {
	pw6ConfigPath(t)
	log.SetLevel("debug")
	defer log.SetLevel("info")
	log.Info("pw6-debug-log-open")
	if err := log.ResetLogsFile(); err != nil {
		t.Fatal(err)
	}
	// The temporary directory gives this invocation a unique, test-only password.
	password := "pw6-test-only-debug-secret-" + filepath.Base(filepath.Dir(filepath.Dir(configFile)))
	if err := WriteConfig(Config{LogLevel: "Debug", SocketProxyPassword: password}); err != nil {
		t.Fatal(err)
	}
	log.Debug("pw6-debug-log-control")
	log.Sync()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "logs", "typonamer.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("pw6-debug-log-control")) {
		t.Fatal("Debug control record missing; log assertion is not exercising Debug level")
	}
	if bytes.Contains(body, []byte(password)) {
		t.Error("Debug log contains socket proxy password")
	}
	if bytes.Contains(body, []byte("New config content:")) || bytes.Contains(body, []byte("SocketProxyPassword:")) {
		t.Error("Debug log contains configuration YAML output")
	}
}
