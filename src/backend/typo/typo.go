package typo

import (
	"fmt"
	"strings"

	"typonamer/config"
	"typonamer/log"
	"typonamer/utils"

	"github.com/duke-git/lancet/v2/strutil"
)

var insertedLetterAlphabetMap = map[string]string{
	"q": "12aw",
	"a": "qwzs",
	"z": "asx",
	"w": "23asqe",
	"s": "wezxad",
	"x": "sdzc",
	"e": "34sdwr",
	"d": "erxcsf",
	"c": "dfxv",
	"r": "45dfet",
	"f": "rtcvdg",
	"v": "fgcb",
	"t": "56fgry",
	"g": "tyvbfh",
	"b": "ghvn",
	"y": "67ghtu",
	"h": "yubngj",
	"n": "hjbm",
	"u": "78hjyi",
	"j": "uinmhk",
	"m": "jkn",
	"i": "89jkuo",
	"k": "iomlj",
	"o": "90klip",
	"l": "opk",
	"p": "0-lo",
	"1": "q2",
	"2": "qw13",
	"3": "we24",
	"4": "er35",
	"5": "rt46",
	"6": "ty57",
	"7": "yu68",
	"8": "ui79",
	"9": "io80",
	"0": "op9",
}

var wrongHorizontalAlphabetMap = map[string]string{
	"q": "w",
	"a": "s",
	"z": "x",
	"w": "qe",
	"s": "ad",
	"x": "zc",
	"e": "wr",
	"d": "sf",
	"c": "xv",
	"r": "et",
	"f": "dg",
	"v": "cb",
	"t": "ry",
	"g": "fh",
	"b": "vn",
	"y": "tu",
	"h": "gj",
	"n": "bm",
	"u": "yi",
	"j": "hk",
	"m": "n",
	"i": "uo",
	"k": "jl",
	"o": "ip",
	"l": "k",
	"p": "o",
	"1": "2",
	"2": "13",
	"3": "24",
	"4": "35",
	"5": "46",
	"6": "57",
	"7": "68",
	"8": "79",
	"9": "80",
	"0": "9",
}

var wrongVerticalAlphabetMap = map[string]string{
	"q": "12a",
	"a": "qwz",
	"z": "as",
	"w": "23as",
	"s": "wezx",
	"x": "sd",
	"e": "34sd",
	"d": "erxc",
	"c": "df",
	"r": "45df",
	"f": "rtcv",
	"v": "fg",
	"t": "56fg",
	"g": "tyvb",
	"b": "gh",
	"y": "67gh",
	"h": "yubn",
	"n": "hj",
	"u": "78hj",
	"j": "uinm",
	"m": "jk",
	"i": "89jk",
	"k": "iom",
	"o": "90kl",
	"l": "op",
	"p": "0-l",
	"1": "q",
	"2": "qw",
	"3": "we",
	"4": "er",
	"5": "rt",
	"6": "ty",
	"7": "yu",
	"8": "ui",
	"9": "io",
	"0": "op",
}

type Typo struct {
	Domain string `json:"domain"`
}

func (t *Typo) TypeWww() []string {
	// 添加www
	return []string{
		fmt.Sprintf("www%s", t.Domain),
	}
}

func (t *Typo) TypeSkipLetter() []string {
	// 跳过字母

	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		newDomain := fmt.Sprintf("%s%s.%s", sld[:i], sld[i+1:], suffix)
		domains = append(domains, newDomain)
	}

	return domains
}

func (t *Typo) TypeDoubleLetter() []string {
	// 添加重复字母

	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		newDomain := fmt.Sprintf("%s%s%s.%s", sld[:i], string(sld[i]), sld[i:], suffix)
		domains = append(domains, newDomain)
	}

	return domains
}

func (t *Typo) TypeReverseLetter() []string {
	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		if i == 0 {
			continue
		}

		// 获取相邻两个字母
		letters := sld[i-1 : i+1]

		// 交换相邻字母
		reversedLetters := string(letters[1]) + string(letters[0])

		// 构建新域名
		newDomain := fmt.Sprintf("%s%s%s.%s", sld[:i-1], reversedLetters, sld[i+1:], suffix)

		domains = append(domains, newDomain)
	}

	return domains
}

func (t *Typo) TypeInsertedLetter() []string {
	// 在字母周围插入字母

	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		for key, value := range insertedLetterAlphabetMap {
			if string(sld[i]) == key {
				for _, item := range value {
					// 在左边插入字母
					leftDomain := fmt.Sprintf("%s%s%s.%s", sld[:i], string(item), sld[i:], suffix)
					domains = append(domains, leftDomain)

					// 在右边插入字母
					rightDomain := fmt.Sprintf("%s%s%s.%s", sld[:i+1], string(item), sld[i+1:], suffix)
					domains = append(domains, rightDomain)
				}
			}
		}
	}

	return domains
}

func (t *Typo) TypeWrongHorizontalKey() []string {
	// 错误的水平按键

	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		for key, value := range wrongHorizontalAlphabetMap {
			if string(sld[i]) == key {
				for _, item := range value {
					newDomain := fmt.Sprintf("%s%s%s.%s", sld[:i], string(item), sld[i+1:], suffix)
					domains = append(domains, newDomain)
				}
			}
		}
	}

	return domains
}

func (t *Typo) TypeWrongVerticalKey() []string {
	// 错误的垂直按键

	domains := []string{}

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	for i := range sld {
		for key, value := range wrongVerticalAlphabetMap {
			if string(sld[i]) == key {
				for _, item := range value {
					newDomain := fmt.Sprintf("%s%s%s.%s", sld[:i], string(item), sld[i+1:], suffix)
					domains = append(domains, newDomain)
				}
			}
		}
	}

	return domains
}

func (t *Typo) TypeWrongTlds(tldList []string) []string {
	// 错误的顶级域名

	domains := []string{}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	formattedTlds := utils.GetFormattedTlds(tldList)

	for _, tld := range formattedTlds {
		newDomain := fmt.Sprintf("%s.%s", sld, tld)
		domains = append(domains, newDomain)
	}

	return domains
}

func (t *Typo) TypeCustomizedReplace() []string {
	// 自定义替换

	domains := []string{}

	cfg := config.GetConfig()

	_, suffix, err := utils.GetTld(t.Domain)
	if err != nil || suffix == "" {
		return domains
	}

	sld := utils.GetSld(t.Domain)
	if sld == "" {
		return domains
	}

	if len(cfg.TypoCustomizedReplaces) == 0 {
		return domains
	}

	for _, item := range cfg.TypoCustomizedReplaces {
		replaceItems := strutil.SplitAndTrim(item, ":")
		if len(replaceItems) != 2 {
			log.Errorf("Wrong typo customized replaces item: %s", item)
			continue
		}

		newSld := strings.ReplaceAll(sld, replaceItems[0], replaceItems[1])

		if newSld != sld {
			newDomain := fmt.Sprintf("%s.%s", newSld, suffix)
			domains = append(domains, newDomain)
		}
	}

	return domains
}
