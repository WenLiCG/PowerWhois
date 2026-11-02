// Prepare a tiny, provenance-preserving live-test workbook from the real 10k
// workbook. This program performs no network requests.
package main

import (
	"fmt"
	"github.com/xuri/excelize/v2"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: online-sample input.xlsx output.xlsx")
	}
	in, e := excelize.OpenFile(os.Args[1])
	if e != nil {
		panic(e)
	}
	defer in.Close()
	out := excelize.NewFile()
	defer out.Close()
	_ = out.SetCellStr("Sheet1", "A1", "Source row in real 10k workbook")
	_ = out.SetCellStr("Sheet1", "B1", "Domain")
	// Keep the first well-known seed and two later actual source records.
	for i, row := range []int{10001, 2, 9998} {
		d, e := in.GetCellValue("Sheet1", fmt.Sprintf("B%d", row))
		if e != nil {
			panic(e)
		}
		_ = out.SetCellInt("Sheet1", fmt.Sprintf("A%d", i+2), int64(row))
		_ = out.SetCellStr("Sheet1", fmt.Sprintf("B%d", i+2), d)
		fmt.Println(row, d)
	}
	if e = out.SaveAs(os.Args[2]); e != nil {
		panic(e)
	}
}
