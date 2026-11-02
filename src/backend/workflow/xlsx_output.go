package workflow

import (
	"archive/zip"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"
)

// Bound disk/output size as well as memory; larger evidence remains available in
// the paginated API. Entry limits avoid Excelize's in-memory ZIP64 patch path.
const maxXLSXBytes int64 = 512 << 20
const maxWorksheetXMLBytes int64 = (2 << 30) - 1

type limitedOutput struct {
	w         io.Writer
	remaining int64
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, fmt.Errorf("XLSX export size limit reached; use paginated API for larger exports")
	}
	n, e := w.w.Write(p)
	w.remaining -= int64(n)
	return n, e
}

type boundedZip struct {
	*zip.Writer
	entryLimit int64
}

func (z *boundedZip) Create(name string) (io.Writer, error) {
	w, e := z.Writer.Create(name)
	if e != nil {
		return nil, e
	}
	return &limitedOutput{w: w, remaining: z.entryLimit}, nil
}

// Excelize v2.11 WriteToBuffer normally accumulates the complete archive even
// for StreamWriter sheets. Its supported ZipWriter factory lets us direct ZIP
// bytes to the destination while the otherwise unused buffer stays empty.
func writeWorkbookStream(f *excelize.File, w io.Writer) error {
	return writeWorkbookWithLimits(f, w, maxXLSXBytes, maxWorksheetXMLBytes)
}
func writeWorkbookWithLimits(f *excelize.File, w io.Writer, archiveLimit, entryLimit int64) error {
	f.SetZipWriter(func(_ io.Writer) excelize.ZipWriter {
		return &boundedZip{Writer: zip.NewWriter(&limitedOutput{w: w, remaining: archiveLimit}), entryLimit: entryLimit}
	})
	return f.Write(io.Discard)
}
