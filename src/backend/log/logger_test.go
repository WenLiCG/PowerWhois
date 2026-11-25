package log

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveAndResetUseWritableRuntimeDirectory(t *testing.T) {
	oldDir, oldFile := logDir, logFile
	t.Cleanup(func() { logDir, logFile = oldDir, oldFile })
	execDir := t.TempDir()
	logDir = filepath.Join(execDir, "logs")
	logFile = filepath.Join(logDir, logFileName)
	os.MkdirAll(logDir, 0700)
	os.WriteFile(logFile, []byte("fixture log"), 0600)
	var archive bytes.Buffer
	if e := WriteZipLogs(&archive); e != nil {
		t.Fatal(e)
	}
	reader, e := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if e != nil {
		t.Fatal(e)
	}
	if len(reader.File) == 0 {
		t.Fatal("empty log download")
	}

	if e = ResetLogsFile(); e != nil {
		t.Fatal(e)
	}
	content, e := os.ReadFile(logFile)
	if e != nil || len(content) != 0 {
		t.Fatal("log reset failed", e)
	}
}
