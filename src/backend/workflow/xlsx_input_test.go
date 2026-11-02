package workflow

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func streamingXLSX(t *testing.T, worksheet, sst string, extra map[string]string) []byte {
	t.Helper()
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Input" sheetId="1" r:id="sheet"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="sheet" Target="worksheets/sheet1.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"/><Relationship Id="sst" Target="sharedStrings.xml" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData>` + worksheet + `</sheetData></worksheet>`,
	}
	if sst != "" {
		parts["xl/sharedStrings.xml"] = `<sst>` + sst + `</sst>`
	}
	for name, value := range extra {
		parts[name] = value
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for name, value := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(w, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func collectStreamingSeeds(data []byte, options ImportOptions) ([]Seed, error) {
	var seeds []Seed
	err := readSeedRows(context.Background(), bytes.NewReader(data), options, func(seed Seed) error {
		seeds = append(seeds, seed)
		return nil
	})
	return seeds, err
}

func TestStreamingXLSXPreservesSelectedInputs(t *testing.T) {
	data := streamingXLSX(t, `
<row r="1"><c r="B1" t="inlineStr"><is><t>Domain</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>999999</v></c><c r="B2" t="s"><v>1</v></c><c r="XFD2" t="inlineStr"><is><t>ignore.example</t></is></c></row>
<row r="4"><c r="B4" t="inlineStr"><is><r><t xml:space="preserve"> EXAMPLE</t></r><rPh><t>phonetic</t></rPh><r><t>.COM </t></r></is></c></row>
<row r="5"><c r="B5"><f>"uncached.example"</f></c></row>
<row r="6"><c r="B6"><f t="shared" si="0"/></c></row>
<row r="7"><c r="B7" t="str"><f>"cached.example"</f><v>cached.example</v></c></row>
<row r="8"><c r="B8" t="inlineStr"><is><t>-bad.example</t></is></c></row>
<row r="9"><c r="B9" t="s"><v>1</v></c></row>`, `<si><t>unused.example</t></si><si><r><t>shared</t></r><r><t>.example</t></r></si>`, nil)
	seeds, err := collectStreamingSeeds(data, ImportOptions{Sheet: "Input", Column: "b", Header: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 7 {
		t.Fatalf("got %d seeds: %+v", len(seeds), seeds)
	}
	want := []struct {
		row         int
		raw, domain string
		invalid     bool
	}{
		{2, "shared.example", "shared.example", false},
		{4, " EXAMPLE.COM ", "example.com", false},
		{5, `="uncached.example"`, "", true},
		{6, "=", "", true},
		{7, `="cached.example"`, "", true},
		{8, "-bad.example", "", true},
		{9, "shared.example", "shared.example", false},
	}
	for i, w := range want {
		s := seeds[i]
		if s.Row != w.row || s.Sheet != "Input" || s.Raw != w.raw || s.Domain != w.domain || (s.Error != "") != w.invalid || s.Duplicate {
			t.Errorf("seed %d: got %+v, want %+v", i, s, w)
		}
	}
}

func TestStreamingXLSXReadsExistingExcelizeFixture(t *testing.T) {
	seeds, err := collectStreamingSeeds(fixture(t, "example.com", "EXAMPLE.COM", "-bad.com"), ImportOptions{Header: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 4 || seeds[0].Domain != "example.com" || seeds[1].Raw != "EXAMPLE.COM" || seeds[2].Error == "" || seeds[3].Raw != "=-bad.example" || seeds[3].Error == "" {
		t.Fatalf("Excelize fixture changed: %+v", seeds)
	}
}

func TestStreamingXLSXIgnoresExternalHyperlinksWithoutFetching(t *testing.T) {
	var fetched atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { fetched.Add(1) }))
	defer server.Close()
	data := streamingXLSX(t, `<row r="1"><c r="B1" t="inlineStr"><is><t>example.com</t></is></c></row>`, "", map[string]string{
		"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Id="link" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" TargetMode="External" Target="` + server.URL + `/?a=1&amp;b=2#fragment"/></Relationships>`,
	})
	seeds, err := collectStreamingSeeds(data, ImportOptions{})
	if err != nil || len(seeds) != 1 || seeds[0].Raw != "example.com" || seeds[0].Domain != "example.com" {
		t.Fatalf("external formatting hyperlink changed input: %+v, %v", seeds, err)
	}
	if fetched.Load() != 0 {
		t.Fatal("XLSX hyperlink target was fetched")
	}
}

func TestStreamingXLSXDecodesLogicalStringEscapesOnce(t *testing.T) {
	if decoded, err := xlsxString("_x+065_xample.com"); err != nil || decoded != "_x+065_xample.com" {
		t.Fatalf("malformed escape changed identity: %q, %v", decoded, err)
	}
	data := streamingXLSX(t, `
<row r="1"><c r="B1" t="inlineStr"><is><t>_x0065_xample.com</t></is></c></row>
<row r="2"><c r="B2" t="s"><v>0</v></c></row>
<row r="3"><c r="B3" t="inlineStr"><is><t>_x005F_x0065_xample.com</t></is></c></row>
<row r="4"><c r="B4" t="s"><v>1</v></c></row>
<row r="5"><c r="B5" t="str"><v>n_x0065_t.com</v></c></row>
<row r="6"><c r="B6" t="str"><f>"_x0065_xample.com"</f><v>_x0065_xample.com</v></c></row>`, `<si><t>_x002D_bad.com</t></si><si><t>_xD83D__xDE00_.com</t></si>`, nil)
	seeds, err := collectStreamingSeeds(data, ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 6 {
		t.Fatalf("got %d seeds: %+v", len(seeds), seeds)
	}
	wantRaw := []string{"example.com", "-bad.com", "_x0065_xample.com", "😀.com", "net.com", `="_x0065_xample.com"`}
	for i, raw := range wantRaw {
		if seeds[i].Raw != raw {
			t.Errorf("row %d: raw=%q want=%q", i+1, seeds[i].Raw, raw)
		}
	}
	if seeds[0].Domain != "example.com" || seeds[4].Domain != "net.com" || seeds[1].Error == "" || seeds[2].Error == "" || seeds[5].Error != "formula cells are invalid domain inputs" {
		t.Fatalf("logical values classified incorrectly: %+v", seeds)
	}
	// Apply Excel's character limit to decoded text, rather than its longer XML
	// escape spelling. Raw text remains available even when it is not a domain.
	long := strings.Repeat("_x0065_", xlsxCellLimit)
	data = streamingXLSX(t, `<row r="1"><c r="B1" t="inlineStr"><is><t>`+long+`</t></is></c></row>`, "", nil)
	seeds, err = collectStreamingSeeds(data, ImportOptions{})
	if err != nil || len(seeds) != 1 || seeds[0].Raw != strings.Repeat("e", xlsxCellLimit) {
		t.Fatalf("decoded character boundary lost: seeds=%d error=%v", len(seeds), err)
	}
}

func TestStreamingXLSXSparseRowsStayBounded(t *testing.T) {
	var rows strings.Builder
	for row := 1; row <= 500; row++ {
		fmt.Fprintf(&rows, `<row r="%d"><c r="B%d" t="inlineStr"><is><t>example.com</t></is></c><c r="XFD%d"><v>1</v></c></row>`, row, row, row)
	}
	data := streamingXLSX(t, rows.String(), "", nil)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	count := 0
	err := readSeedRows(context.Background(), bytes.NewReader(data), ImportOptions{}, func(Seed) error { count++; return nil })
	runtime.ReadMemStats(&after)
	if err != nil || count != 500 {
		t.Fatalf("sparse import count=%d err=%v", count, err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32<<20 {
		t.Fatalf("sparse columns caused dense allocation: %d bytes", allocated)
	}
	data = streamingXLSX(t, `<row r="1048576"><c r="XFD1048576"><v>1</v></c></row>`, "", nil)
	if _, err = collectStreamingSeeds(data, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "100000 rows") {
		t.Fatalf("high sparse row was not rejected early: %v", err)
	}
}

func TestStreamingXLSXValidation(t *testing.T) {
	cases := []struct {
		name, rows, sst, want string
		extra                 map[string]string
	}{
		{"duplicate row", `<row r="2"/><row r="2"/>`, "", "increasing", nil},
		{"reversed column", `<row r="1"><c r="C1"/><c r="B1"/></row>`, "", "increasing", nil},
		{"mismatched row", `<row r="1"><c r="B2"/></row>`, "", "coordinate", nil},
		{"invalid coordinate", `<row r="1"><c r="B+1"/></row>`, "", "coordinate", nil},
		{"invalid row", `<row r="+1"/>`, "", "coordinate", nil},
		{"duplicate attribute", `<row r="1" r="2"/>`, "", "duplicate", nil},
		{"column overflow", `<row r="1"><c r="XFE1"/></row>`, "", "coordinate", nil},
		{"missing shared index", `<row r="1"><c r="B1" t="s"><v>2</v></c></row>`, `<si><t>example.com</t></si>`, "out of range", nil},
		{"missing table", `<row r="1"><c r="B1" t="s"><v>0</v></c></row>`, "", "missing", nil},
		{"external worksheet", "", "", "external", map[string]string{"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Id="evil" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="https://evil.test/" TargetMode="External"/></Relationships>`}},
		{"external shared strings", "", "", "external", map[string]string{"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Id="evil" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="https://evil.test/" TargetMode="External"/></Relationships>`}},
		{"external workbook", "", "", "external", map[string]string{"_rels/.rels": `<Relationships><Relationship Id="evil" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="https://evil.test/" TargetMode="External"/></Relationships>`}},
		{"hyperlink traversal", "", "", "path traversal", map[string]string{"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Id="evil" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="../../outside" TargetMode="External"/></Relationships>`}},
		{"path traversal", "", "", "path", map[string]string{"../payload": "x"}},
		{"large selected cell", `<row r="1"><c r="B1" t="inlineStr"><is><t>` + strings.Repeat("x", xlsxCellLimit+1) + `</t></is></c></row>`, "", "32767", nil},
		{"large XML token", `<row r="1"><c r="A1" t="inlineStr"><is><t>` + strings.Repeat("x", (1<<20)+1) + `</t></is></c></row>`, "", "token", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collectStreamingSeeds(streamingXLSX(t, tc.rows, tc.sst, tc.extra), ImportOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q error, got %v", tc.want, err)
			}
		})
	}
	for _, tc := range []struct {
		header bool
		row    int
		valid  bool
	}{{false, 100000, true}, {false, 100001, false}, {true, 100001, true}, {true, 100002, false}} {
		data := streamingXLSX(t, fmt.Sprintf(`<row r="%d"><c r="B%d" t="inlineStr"><is><t>example.com</t></is></c></row>`, tc.row, tc.row), "", nil)
		_, err := collectStreamingSeeds(data, ImportOptions{Header: tc.header})
		if (err == nil) != tc.valid {
			t.Fatalf("row=%d header=%v: %v", tc.row, tc.header, err)
		}
	}
}

func TestStreamingXLSXTextAndArchiveBudgets(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= 1025; i++ {
		fmt.Fprintf(&rows, `<row r="%d"><c r="B%d" t="s"><v>0</v></c></row>`, i, i)
	}
	data := streamingXLSX(t, rows.String(), `<si><t>`+strings.Repeat("x", xlsxCellLimit)+`</t></si>`, nil)
	if _, err := collectStreamingSeeds(data, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "raw text exceeds") {
		t.Fatalf("repeated SST value bypassed aggregate budget: %v", err)
	}
	data = streamingXLSX(t, "", "", nil)
	// Forge the central directory's first declared uncompressed size. Validation
	// must reject it before attempting XML parsing or decompression.
	central := bytes.Index(data, []byte{'P', 'K', 1, 2})
	if central < 0 {
		t.Fatal("central directory missing")
	}
	binary.LittleEndian.PutUint32(data[central+24:central+28], xlsxUnzipLimit+1)
	if _, err := collectStreamingSeeds(data, ImportOptions{}); err == nil || !strings.Contains(err.Error(), "decompressed size") {
		t.Fatalf("declared decompression limit ignored: %v", err)
	}
	if err := readSeedRows(context.Background(), io.LimitReader(zeroXLSXReader{}, xlsxUploadLimit+1), ImportOptions{}, func(Seed) error { return nil }); err == nil || !strings.Contains(err.Error(), "upload exceeds") {
		t.Fatalf("upload limit ignored: %v", err)
	}
}

type zeroXLSXReader struct{}

func (zeroXLSXReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestStreamingXLSXCancellationAndYieldError(t *testing.T) {
	data := streamingXLSX(t, `<row r="1"><c r="B1" t="inlineStr"><is><t>example.com</t></is></c></row>`, "", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := readSeedRows(ctx, bytes.NewReader(data), ImportOptions{}, func(Seed) error { t.Fatal("yield after cancellation"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	want := errors.New("stop importing")
	if err := readSeedRows(context.Background(), bytes.NewReader(data), ImportOptions{}, func(Seed) error { return want }); !errors.Is(err, want) {
		t.Fatalf("yield failure lost: %v", err)
	}
}
