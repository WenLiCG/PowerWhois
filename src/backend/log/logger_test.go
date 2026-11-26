package log

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestArchiveAndResetUseWritableRuntimeDirectory(t *testing.T) {
	oldWriter := logWriter
	logDir := t.TempDir()
	logFile := filepath.Join(logDir, logFileName)
	logWriter = &lumberjack.Logger{Filename: logFile}
	t.Cleanup(func() { logWriter.Close(); logWriter = oldWriter })
	if _, err := logWriter.Write([]byte("fixture log")); err != nil {
		t.Fatal(err)
	}
	rotation := "typonamer-2026-01-02T03-04-05.678.log"
	unrelated := []string{"other-app.log", "typonamer-debug.log", "typonamer-2026-01-02T03-04-05,678.log"}
	for _, name := range append([]string{rotation}, unrelated...) {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(logDir, "typonamer-2026-01-03T03-04-05.678.log"), 0700); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if e := WriteZipLogs(&archive); e != nil {
		t.Fatal(e)
	}
	reader, e := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if e != nil {
		t.Fatal(e)
	}
	if len(reader.File) != 2 || reader.File[0].Name != rotation || reader.File[1].Name != logFileName {
		t.Fatalf("archive contains unexpected entries: %v", reader.File)
	}
	current, err := reader.File[1].Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(current)
	current.Close()
	if err != nil || string(content) != "fixture log" {
		t.Fatalf("incorrect archived log %q: %v", content, err)
	}

	if e = ResetLogsFile(); e != nil {
		t.Fatal(e)
	}
	if _, err := logWriter.Write([]byte("after reset")); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(logFile)
	if err != nil || string(content) != "after reset" {
		t.Fatalf("incorrect log after reset %q: %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(logDir, rotation)); !os.IsNotExist(err) {
		t.Fatal("rotation was not removed", err)
	}
	for _, name := range unrelated {
		if b, err := os.ReadFile(filepath.Join(logDir, name)); err != nil || string(b) != name {
			t.Fatal("reset changed an unrelated log", name, err)
		}
	}
}
