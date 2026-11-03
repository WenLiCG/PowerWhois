package log

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveAndResetUseWritableRuntimeDirectory(t *testing.T) {
	oldExec, oldDir, oldFile := execDir, logDir, logFile
	t.Cleanup(func() { execDir, logDir, logFile = oldExec, oldDir, oldFile })
	execDir = t.TempDir()
	logDir = filepath.Join(execDir, "logs")
	logFile = filepath.Join(logDir, logFileName)
	os.MkdirAll(logDir, 0700)
	os.WriteFile(logFile, []byte("fixture log"), 0600)
	archive, e := GetZipLogsFile()
	if e != nil || filepath.Dir(archive) != execDir {
		t.Fatal(archive, e)
	}
	reader, e := zip.OpenReader(archive)
	if e != nil {
		t.Fatal(e)
	}
	if len(reader.File) == 0 {
		t.Fatal("empty log download")
	}
	reader.Close()
	if e = ResetLogsFile(); e != nil {
		t.Fatal(e)
	}
	content, e := os.ReadFile(logFile)
	if e != nil || len(content) != 0 {
		t.Fatal("log reset failed", e)
	}
}
