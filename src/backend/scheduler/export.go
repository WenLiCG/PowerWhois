package scheduler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"typonamer/constant"
	"typonamer/lookup/lookupinfo"

	"github.com/jszwec/csvutil"
)

func (b *BulkCheck) Export(ctx context.Context, output io.Writer) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != nil {
		return ErrBulkState
	}
	status, err := b.status(ctx)
	if err != nil {
		return err
	}
	if status != constant.BulkCheckStatusDone && status != constant.BulkCheckStatusPaused &&
		status != constant.BulkCheckStatusCanceled && status != constant.BulkCheckStatusError && b.failure == "" {
		return ErrBulkState
	}

	// ponytail: sort only export columns in memory. Add a disk merge if export exceeds the VPS memory budget.
	type row struct {
		order int
		data  lookupinfo.QueryCsvResult
	}
	var rows []row
	for _, key := range []string{takenKey, freeKey, errorKey} {
		for start := int64(0); ; start += pageSize {
			items, err := b.redis.LRange(ctx, key, start, start+pageSize-1).Result()
			if err != nil {
				return err
			}
			for _, item := range items {
				var result lookupinfo.QueryResult
				if err := json.Unmarshal([]byte(item), &result); err != nil || result.Domain == "" ||
					result.RegisterStatus == "" || result.Order < 0 {
					return errors.New("invalid saved query result")
				}
				rows = append(rows, row{result.Order, csvRow(result)})
			}
			if len(items) < pageSize {
				break
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].order < rows[j].order })

	if _, err := io.WriteString(output, "\uFEFF"); err != nil {
		return err
	}
	writer := csv.NewWriter(output)
	header, err := csvutil.Header(lookupinfo.QueryCsvResult{}, "csv")
	if err != nil {
		return err
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	encoder := csvutil.NewEncoder(writer)
	encoder.AutoHeader = false
	for i := range rows {
		if err := encoder.Encode(rows[i].data); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func csvRow(result lookupinfo.QueryResult) lookupinfo.QueryCsvResult {
	// Older DNS tasks recorded an absent delegation as Free.
	if result.LookupType == constant.LookupTypeDNS && result.RegisterStatus == constant.DomainRegisterStatusFree {
		result.RegisterStatus = constant.DomainRegisterStatusUnknown
		result.QueryError = "旧版 DNS 记录无法确认注册状态，请重新查询"
	}
	proxy := ""
	if result.LookupType == constant.LookupTypeWhois || result.LookupType == constant.LookupTypeRDAP {
		proxy = "No"
		if result.ViaProxy {
			proxy = "Yes"
		}
	}
	return lookupinfo.QueryCsvResult{
		Domain: safeCell(result.Domain), LookupType: safeCell(result.LookupType), ViaProxy: proxy,
		Status: safeCell(result.RegisterStatus), ErrorInfo: safeCell(result.QueryError),
		CreatedDate: safeCell(result.CreatedDate), ExpiryDate: safeCell(result.ExpiryDate),
		NameServer: safeCell(strings.Join(result.NameServer, ",")), DnsLite: safeCell(result.DnsLite),
		RawDomainStatus: safeCell(strings.Join(result.RawDomainStatus, ",")), DomainStatus: safeCell(result.DomainStatus),
	}
}

func safeCell(s string) string {
	first := strings.TrimLeftFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\uFEFF' })
	head, _ := utf8.DecodeRuneInString(first)
	if strings.ContainsRune("=+-@＝＋－＠", head) {
		return "'" + s
	}
	if len(s) != 0 && strings.ContainsRune("\t\r\n", rune(s[0])) {
		return "'" + s
	}
	return s
}
