package typo

import (
	"errors"

	"typonamer/constant"
	"typonamer/utils"
)

type Group struct {
	Kind    string
	Domains []string
}

func Generate(input string, kinds, suffixes []string, limit int) ([]Group, []string, error) {
	domain, err := utils.TrimAndGetMainDomain(input)
	if err != nil || len(kinds) > 8 || len(suffixes) > 100 {
		return nil, nil, errors.New("Typo 域名或选项无效")
	}
	t := Typo{Domain: domain}
	generators := map[string]func() []string{
		constant.TypoTypeWww: t.TypeWww, constant.TypoTypeSkipLetter: t.TypeSkipLetter,
		constant.TypoTypeDoubleLetter: t.TypeDoubleLetter, constant.TypoTypeReverseLetter: t.TypeReverseLetter,
		constant.TypoTypeInsertedLetter: t.TypeInsertedLetter, constant.TypoTypeWrongHorizontalKey: t.TypeWrongHorizontalKey,
		constant.TypoTypeWrongVerticalKey: t.TypeWrongVerticalKey, constant.TypoTypeCustomizedReplace: t.TypeCustomizedReplace,
		constant.TypoTypeWrongTlds: func() []string { return t.TypeWrongTlds(suffixes) },
	}
	var groups []Group
	var domains []string
	seen := map[string]bool{domain: true}
	generated := make(map[string]bool)
	selected := append([]string{}, kinds...)
	if len(suffixes) > 0 {
		selected = append(selected, constant.TypoTypeWrongTlds)
	}
	for _, kind := range selected {
		generate, ok := generators[kind]
		if !ok {
			return nil, nil, errors.New("未知的 Typo 类型")
		}
		if generated[kind] {
			continue
		}
		generated[kind] = true
		group := Group{Kind: kind}
		for _, candidate := range generate() {
			name, err := utils.TrimDomain(candidate)
			if err != nil || seen[name] {
				continue
			}
			canonical, err := utils.GetDomainSuffixPlusOne(name)
			if err != nil || canonical != name {
				continue
			}
			seen[name] = true
			domains = append(domains, name)
			group.Domains = append(group.Domains, name)
			if len(domains) > limit {
				return nil, nil, errors.New("生成域名超过单批限制，请减少 Typo 选项")
			}
		}
		if len(group.Domains) > 0 {
			groups = append(groups, group)
		}
	}
	if len(domains) == 0 {
		return nil, nil, errors.New("没有生成有效的 Typo 域名")
	}
	return groups, domains, nil
}
