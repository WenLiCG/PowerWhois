package workflow

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	xlsxUploadLimit = 32 << 20
	xlsxUnzipLimit  = 256 << 20
	xlsxRawLimit    = 32 << 20
	xlsxCellLimit   = 32767
)

type xlsxInput struct {
	ctx   context.Context
	files map[string]*zip.File
}

// XML's decoder otherwise permits an arbitrarily large text/attribute token.
// Providing ReadByte lets us stop a token before the decoder allocates it all.
type xlsxXML struct {
	ctx   context.Context
	input *bufio.Reader
	read  int
	dec   *xml.Decoder
	depth int
	roots int
}

func newXLSXXML(ctx context.Context, r io.Reader) *xlsxXML {
	x := &xlsxXML{ctx: ctx, input: bufio.NewReader(r)}
	x.dec = xml.NewDecoder(x)
	return x
}
func (x *xlsxXML) Read(p []byte) (int, error) {
	if err := x.ctx.Err(); err != nil {
		return 0, err
	}
	if x.read >= 1<<20 {
		return 0, fmt.Errorf("XLSX XML token exceeds 1 MiB")
	}
	if len(p) > (1<<20)-x.read {
		p = p[:(1<<20)-x.read]
	}
	n, err := x.input.Read(p)
	x.read += n
	return n, err
}
func (x *xlsxXML) ReadByte() (byte, error) {
	if err := x.ctx.Err(); err != nil {
		return 0, err
	}
	if x.read >= 1<<20 {
		return 0, fmt.Errorf("XLSX XML token exceeds 1 MiB")
	}
	x.read++
	return x.input.ReadByte()
}
func (x *xlsxXML) next() (xml.Token, error) {
	x.read = 0
	t, err := x.dec.Token()
	if err == io.EOF && (x.depth != 0 || x.roots != 1) {
		return nil, fmt.Errorf("invalid XLSX XML document")
	}
	if _, ok := t.(xml.Directive); ok {
		return nil, fmt.Errorf("XLSX XML directives are not supported")
	}
	switch t.(type) {
	case xml.StartElement:
		e := t.(xml.StartElement)
		if len(e.Attr) > 256 {
			return nil, fmt.Errorf("too many XLSX XML attributes")
		}
		attributes := map[xml.Name]bool{}
		for _, attr := range e.Attr {
			if attributes[attr.Name] {
				return nil, fmt.Errorf("duplicate XLSX XML attribute %q", attr.Name.Local)
			}
			attributes[attr.Name] = true
		}
		if x.depth == 0 {
			x.roots++
			if x.roots != 1 {
				return nil, fmt.Errorf("multiple XLSX XML roots")
			}
		}
		x.depth++
		if x.depth > 256 {
			return nil, fmt.Errorf("XLSX XML nesting exceeds 256 elements")
		}
	case xml.EndElement:
		x.depth--
	case xml.CharData:
		if x.depth == 0 && strings.TrimSpace(string(t.(xml.CharData))) != "" {
			return nil, fmt.Errorf("text outside XLSX XML root")
		}
	}
	return t, err
}
func (x *xlsxXML) root(name string) error {
	for {
		t, err := x.next()
		if err != nil {
			return err
		}
		if e, ok := t.(xml.StartElement); ok {
			if e.Name.Local != name {
				return fmt.Errorf("expected XLSX %s document", name)
			}
			return nil
		}
	}
}
func (x *xlsxXML) skip() error {
	depth := 1
	for depth > 0 {
		t, err := x.next()
		if err != nil {
			return err
		}
		switch t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}
func xlsxAttr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (a *xlsxInput) part(name string, parse func(*xlsxXML) error) error {
	f := a.files[name]
	if f == nil {
		return fmt.Errorf("XLSX part %q is missing", name)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	return parse(newXLSXXML(a.ctx, r))
}

type xlsxRelationship struct{ target, kind string }

func xlsxRelationships(x *xlsxXML) (map[string]xlsxRelationship, error) {
	out := map[string]xlsxRelationship{}
	if err := x.root("Relationships"); err != nil {
		return nil, err
	}
	for {
		t, err := x.next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		e, ok := t.(xml.StartElement)
		if !ok || e.Name.Local != "Relationship" || x.depth != 2 {
			continue
		}
		id, target := xlsxAttr(e, "Id"), xlsxAttr(e, "Target")
		if _, exists := out[id]; id == "" || exists || len(out) >= 10000 {
			return nil, fmt.Errorf("invalid or excessive XLSX relationships")
		}
		u, err := url.Parse(target)
		mode := xlsxAttr(e, "TargetMode")
		kind := xlsxAttr(e, "Type")
		if err == nil && mode == "External" && strings.HasSuffix(kind, "/hyperlink") {
			if strings.ContainsAny(target, "\\\x00") {
				return nil, fmt.Errorf("invalid XLSX hyperlink path")
			}
			for _, component := range strings.Split(u.Path, "/") {
				if component == ".." {
					return nil, fmt.Errorf("invalid XLSX hyperlink path traversal")
				}
			}
			// Automatic Excel hyperlinks are formatting only. Retain the ID for
			// duplicate detection, but never retain or resolve their destinations.
			out[id] = xlsxRelationship{kind: kind}
			continue
		}
		if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path == "" || (mode != "" && mode != "Internal") {
			return nil, fmt.Errorf("external or invalid XLSX relationship %q", id)
		}
		out[id] = xlsxRelationship{target: u.Path, kind: kind}
	}
}
func xlsxTarget(base, target string) (string, error) {
	if strings.ContainsAny(target, "\\\x00") {
		return "", fmt.Errorf("invalid XLSX part path")
	}
	name := path.Clean(path.Join(path.Dir(base), target))
	if strings.HasPrefix(target, "/") {
		name = strings.TrimPrefix(path.Clean(target), "/")
	}
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("XLSX relationship escapes package")
	}
	return name, nil
}

// Validate every member, including unused parts, so forged ZIP size fields or
// unrelated compressed payloads cannot evade the decompression budget.
func (a *xlsxInput) validate(z *zip.Reader) error {
	if len(z.File) > 10000 {
		return fmt.Errorf("XLSX contains too many ZIP members")
	}
	var declared uint64
	for _, f := range z.File {
		if f.UncompressedSize64 > xlsxUnzipLimit-declared {
			return fmt.Errorf("XLSX decompressed size exceeds 256 MiB")
		}
		declared += f.UncompressedSize64
		name := f.Name
		if strings.HasSuffix(name, "/") {
			continue
		}
		if name == "." || name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00") {
			return fmt.Errorf("invalid XLSX ZIP member path")
		}
		if a.files[name] != nil {
			return fmt.Errorf("duplicate XLSX ZIP member %q", name)
		}
		a.files[name] = f
	}
	var actual int64
	for _, f := range z.File {
		if err := a.ctx.Err(); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(io.Discard, io.LimitReader(&xlsxContextReader{a.ctx, r}, xlsxUnzipLimit-actual+1))
		closeErr := r.Close()
		actual += n
		if actual > xlsxUnzipLimit {
			return fmt.Errorf("XLSX decompressed size exceeds 256 MiB")
		}
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if strings.HasSuffix(f.Name, ".rels") {
			if err = a.part(f.Name, func(x *xlsxXML) error { _, e := xlsxRelationships(x); return e }); err != nil {
				return err
			}
		}
	}
	return nil
}

type xlsxContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *xlsxContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (a *xlsxInput) worksheet(wanted string) (sheet, member, sst string, err error) {
	const book = "xl/workbook.xml"
	var rels map[string]xlsxRelationship
	err = a.part("xl/_rels/workbook.xml.rels", func(x *xlsxXML) error { var e error; rels, e = xlsxRelationships(x); return e })
	if err != nil {
		return
	}
	for _, rel := range rels {
		if strings.HasSuffix(rel.kind, "/sharedStrings") {
			if sst != "" {
				return "", "", "", fmt.Errorf("duplicate shared-string relationship")
			}
			sst, err = xlsxTarget(book, rel.target)
			if err != nil {
				return
			}
		}
	}
	err = a.part(book, func(x *xlsxXML) error {
		if e := x.root("workbook"); e != nil {
			return e
		}
		seen := map[string]bool{}
		for {
			t, e := x.next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			n, ok := t.(xml.StartElement)
			if !ok || n.Name.Local != "sheet" || x.depth != 3 {
				continue
			}
			name := xlsxAttr(n, "name")
			if name == "" || seen[name] || len(seen) >= 10000 {
				return fmt.Errorf("invalid or excessive XLSX worksheets")
			}
			seen[name] = true
			if sheet != "" || (wanted != "" && name != wanted) {
				continue
			}
			rel, exists := rels[xlsxAttr(n, "id")]
			if !exists || !strings.HasSuffix(rel.kind, "/worksheet") {
				return fmt.Errorf("selected sheet is not a worksheet")
			}
			sheet = name
			member, e = xlsxTarget(book, rel.target)
			if e != nil {
				return e
			}
		}
		if sheet == "" {
			return fmt.Errorf("worksheet %q not found", wanted)
		}
		return nil
	})
	return
}

func xlsxColumn(name string) (int, error) {
	column := 0
	for _, c := range strings.ToUpper(name) {
		if c < 'A' || c > 'Z' || column > 16384 {
			return 0, fmt.Errorf("invalid XLSX column %q", name)
		}
		column = column*26 + int(c-'A'+1)
	}
	if column < 1 || column > 16384 {
		return 0, fmt.Errorf("invalid XLSX column %q", name)
	}
	return column, nil
}
func xlsxCoordinate(ref string) (column, row int, err error) {
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		i++
	}
	column, err = xlsxColumn(ref[:i])
	if err != nil || i == len(ref) || ref[i] == '0' {
		return 0, 0, fmt.Errorf("invalid XLSX cell coordinate %q", ref)
	}
	for _, digit := range ref[i:] {
		if digit < '0' || digit > '9' {
			return 0, 0, fmt.Errorf("invalid XLSX cell coordinate %q", ref)
		}
	}
	row, err = strconv.Atoi(ref[i:])
	if err != nil || row < 1 || row > 1048576 {
		return 0, 0, fmt.Errorf("invalid XLSX cell coordinate %q", ref)
	}
	return
}

// Decode SpreadsheetML UTF-16 escapes once. An escaped underscore therefore
// cannot turn the following literal xHHHH_ text into another escape.
func xlsxString(value string) (string, error) {
	unit := func(at int) (uint16, bool) {
		if at+7 > len(value) || value[at] != '_' || value[at+1] != 'x' || value[at+6] != '_' {
			return 0, false
		}
		for _, c := range value[at+2 : at+6] {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return 0, false
			}
		}
		n, err := strconv.ParseUint(value[at+2:at+6], 16, 16)
		return uint16(n), err == nil
	}
	if strings.Contains(value, "_x") {
		var decoded strings.Builder
		for i := 0; i < len(value); {
			u, ok := unit(i)
			if !ok {
				decoded.WriteByte(value[i])
				i++
				continue
			}
			i += 7
			r := rune(u)
			if u >= 0xD800 && u <= 0xDBFF {
				if low, paired := unit(i); paired && low >= 0xDC00 && low <= 0xDFFF {
					r = utf16.DecodeRune(r, rune(low))
					i += 7
				} else {
					r = utf8.RuneError
				}
			} else if u >= 0xDC00 && u <= 0xDFFF {
				r = utf8.RuneError
			}
			decoded.WriteRune(r)
		}
		value = decoded.String()
	}
	if utf8.RuneCountInString(value) > xlsxCellLimit {
		return "", fmt.Errorf("XLSX selected cell exceeds 32767 characters")
	}
	return value, nil
}

// Gather rich text without phonetic annotations. For unused SST entries only
// tokens are consumed; their text is never retained.
func xlsxText(x *xlsxXML, collect bool) (string, error) {
	var text strings.Builder
	depth, textDepth, phonetic := 1, 0, 0
	chars := 0
	for depth > 0 {
		t, err := x.next()
		if err != nil {
			return "", err
		}
		switch v := t.(type) {
		case xml.StartElement:
			depth++
			if v.Name.Local == "rPh" {
				phonetic = depth
			}
			if v.Name.Local == "t" && phonetic == 0 {
				textDepth = depth
			}
		case xml.EndElement:
			if depth == textDepth {
				textDepth = 0
			}
			if depth == phonetic {
				phonetic = 0
			}
			depth--
		case xml.CharData:
			if collect && textDepth != 0 {
				chars += utf8.RuneCount(v)
				// Each logical character can use two escaped UTF-16 units.
				if chars > xlsxCellLimit*14 {
					return "", fmt.Errorf("XLSX selected cell exceeds 32767 characters")
				}
				text.Write(v)
			}
		}
	}
	return xlsxString(text.String())
}
func xlsxValue(x *xlsxXML, maxChars int) (string, error) {
	var text strings.Builder
	chars := 0
	for {
		t, err := x.next()
		if err != nil {
			return "", err
		}
		switch v := t.(type) {
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("unexpected element in XLSX cell value")
		case xml.CharData:
			chars += utf8.RuneCount(v)
			if chars > maxChars {
				return "", fmt.Errorf("XLSX selected cell exceeds 32767 characters")
			}
			text.Write(v)
		}
	}
}

type xlsxSelectedCell struct {
	Row     int
	Raw     string
	Shared  *int `json:",omitempty"`
	Formula bool
}

func xlsxCell(x *xlsxXML, start xml.StartElement, row int) (xlsxSelectedCell, error) {
	c := xlsxSelectedCell{Row: row}
	value, inline, formula := "", "", ""
	seen := map[string]bool{}
	for {
		t, err := x.next()
		if err != nil {
			return c, err
		}
		if _, ok := t.(xml.EndElement); ok {
			break
		}
		e, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		if seen[e.Name.Local] {
			return c, fmt.Errorf("duplicate XLSX cell element %q", e.Name.Local)
		}
		seen[e.Name.Local] = true
		switch e.Name.Local {
		case "v":
			maxChars := xlsxCellLimit
			if xlsxAttr(start, "t") == "str" {
				maxChars *= 14
			}
			value, err = xlsxValue(x, maxChars)
		case "f":
			c.Formula = true
			formula, err = xlsxValue(x, xlsxCellLimit)
		case "is":
			inline, err = xlsxText(x, true)
		default:
			err = x.skip()
		}
		if err != nil {
			return c, err
		}
	}
	if c.Formula {
		c.Raw = "=" + formula
		if utf8.RuneCountInString(c.Raw) > xlsxCellLimit {
			return c, fmt.Errorf("XLSX selected cell exceeds 32767 characters")
		}
		return c, nil
	}
	switch xlsxAttr(start, "t") {
	case "s":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return c, fmt.Errorf("invalid XLSX shared-string index")
		}
		c.Shared = &n
	case "inlineStr":
		c.Raw = inline
	case "str":
		decoded, err := xlsxString(value)
		if err != nil {
			return c, err
		}
		c.Raw = decoded
	case "", "n", "b", "e", "d":
		c.Raw = value
	default:
		return c, fmt.Errorf("unsupported XLSX cell type %q", xlsxAttr(start, "t"))
	}
	return c, nil
}

func xlsxRows(x *xlsxXML, column int, header bool, selected func(xlsxSelectedCell) error) error {
	lastRow, inData := 0, false
	seenData := false
	if err := x.root("worksheet"); err != nil {
		return err
	}
	maxRow := 100000
	if header {
		maxRow++
	}
	for {
		t, err := x.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch v := t.(type) {
		case xml.EndElement:
			if v.Name.Local == "sheetData" {
				inData = false
			}
		case xml.StartElement:
			if v.Name.Local == "sheetData" && x.depth == 2 {
				if seenData {
					return fmt.Errorf("duplicate XLSX sheetData")
				}
				seenData = true
				inData = true
			}
			if v.Name.Local != "row" || !inData || x.depth != 3 {
				continue
			}
			row := lastRow + 1
			if raw := xlsxAttr(v, "r"); raw != "" {
				for _, digit := range raw {
					if digit < '0' || digit > '9' {
						return fmt.Errorf("invalid XLSX row coordinate %q", raw)
					}
				}
				row, err = strconv.Atoi(raw)
				if err != nil || row < 1 {
					return fmt.Errorf("invalid XLSX row coordinate %q", raw)
				}
			}
			if row <= lastRow {
				return fmt.Errorf("XLSX rows must have increasing coordinates")
			}
			if row > maxRow {
				return fmt.Errorf("input exceeds 100000 rows")
			}
			lastRow = row
			lastCol := 0
			for {
				t, err = x.next()
				if err != nil {
					return err
				}
				if _, ok := t.(xml.EndElement); ok {
					break
				}
				cell, ok := t.(xml.StartElement)
				if !ok {
					continue
				}
				if cell.Name.Local != "c" {
					if err = x.skip(); err != nil {
						return err
					}
					continue
				}
				col := lastCol + 1
				if ref := xlsxAttr(cell, "r"); ref != "" {
					var cellRow int
					col, cellRow, err = xlsxCoordinate(ref)
					if err != nil || cellRow != row {
						return fmt.Errorf("invalid XLSX cell coordinate %q for row %d", ref, row)
					}
				}
				if col <= lastCol || col > 16384 {
					return fmt.Errorf("XLSX cells must have increasing valid column coordinates")
				}
				lastCol = col
				if col != column || (header && row == 1) {
					if err = x.skip(); err != nil {
						return err
					}
					continue
				}
				c, err := xlsxCell(x, cell, row)
				if err != nil {
					return err
				}
				if err = selected(c); err != nil {
					return err
				}
			}
		}
	}
}

// readSeedRows keeps the ZIP and selected row spool on disk. Only requested
// shared strings are retained, bounded by the selected-column text budget.
func readSeedRows(ctx context.Context, r io.Reader, options ImportOptions, yield func(Seed) error) error {
	if options.Column == "" {
		options.Column = "B"
	}
	column, err := xlsxColumn(options.Column)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "powerwhois-xlsx-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	archive, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return err
	}
	defer archive.Close()
	size, err := io.Copy(archive, io.LimitReader(&xlsxContextReader{ctx, r}, xlsxUploadLimit+1))
	if err != nil {
		return err
	}
	if size > xlsxUploadLimit {
		return fmt.Errorf("XLSX upload exceeds 32 MiB")
	}
	z, err := zip.NewReader(archive, size)
	if err != nil {
		return err
	}
	a := &xlsxInput{ctx: ctx, files: map[string]*zip.File{}}
	if err = a.validate(z); err != nil {
		return err
	}
	sheet, member, sst, err := a.worksheet(options.Sheet)
	if err != nil {
		return err
	}
	spool, err := os.CreateTemp(dir, "rows-*")
	if err != nil {
		return err
	}
	defer spool.Close()
	encoder := json.NewEncoder(spool)
	uses, shared := map[int]int{}, map[int]string{}
	rawBytes := 0
	budget := func(text string, count int) error {
		if count > 0 && len(text) > (xlsxRawLimit-rawBytes)/count {
			return fmt.Errorf("XLSX selected raw text exceeds 32 MiB")
		}
		rawBytes += len(text) * count
		return nil
	}
	err = a.part(member, func(x *xlsxXML) error {
		return xlsxRows(x, column, options.Header, func(c xlsxSelectedCell) error {
			if c.Shared != nil {
				uses[*c.Shared]++
				if len(uses) > 100000 {
					return fmt.Errorf("too many required XLSX shared strings")
				}
			} else if err := budget(c.Raw, 1); err != nil {
				return err
			}
			return encoder.Encode(c)
		})
	})
	if err != nil {
		return err
	}
	if len(uses) > 0 {
		if sst == "" {
			return fmt.Errorf("XLSX shared-string table is missing")
		}
		err = a.part(sst, func(x *xlsxXML) error {
			if err := x.root("sst"); err != nil {
				return err
			}
			index := 0
			for {
				t, err := x.next()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				e, ok := t.(xml.StartElement)
				if !ok || e.Name.Local != "si" || x.depth != 2 {
					continue
				}
				text, err := xlsxText(x, uses[index] > 0)
				if err != nil {
					return err
				}
				if count := uses[index]; count > 0 {
					if err = budget(text, count); err != nil {
						return err
					}
					shared[index] = text
				}
				index++
			}
		})
		if err != nil {
			return err
		}
		if len(shared) != len(uses) {
			return fmt.Errorf("XLSX shared-string index is out of range")
		}
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return err
	}
	decoder := json.NewDecoder(spool)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		var cell xlsxSelectedCell
		if err = decoder.Decode(&cell); err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if cell.Shared != nil {
			cell.Raw = shared[*cell.Shared]
		}
		if cell.Raw == "" && !cell.Formula {
			continue
		}
		seed := Seed{Row: cell.Row, Sheet: sheet, Raw: cell.Raw}
		if cell.Formula {
			seed.Error = "formula cells are invalid domain inputs"
		} else {
			seed.Domain, err = NormalizeSeed(seed.Raw)
			if err != nil {
				seed.Error = err.Error()
			}
		}
		if err = yield(seed); err != nil {
			return err
		}
	}
}
