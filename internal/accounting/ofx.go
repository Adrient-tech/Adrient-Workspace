package accounting

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	ofxBlock = regexp.MustCompile(`(?is)<STMTTRN>(.*?)(?:</STMTTRN>|<STMTTRN>|</BANKTRANLIST>|</CCSTMTRS>|$)`)
	ofxTag   = regexp.MustCompile(`(?is)<([A-Z0-9.]+)>([^<\r\n]*)`)
)

// ParseOFX reads statement lines from an OFX or QFX file (SGML 1.x or XML 2.x).
// Amounts keep the file's sign: negative = money out, as in the CSV importer.
func ParseOFX(data string) ([]StatementLine, error) {
	if !strings.Contains(strings.ToUpper(data), "<STMTTRN>") {
		return nil, invalid("no transactions found: this does not look like an OFX or QFX bank file")
	}
	var out []StatementLine
	for _, m := range ofxBlock.FindAllStringSubmatch(data, -1) {
		tags := map[string]string{}
		for _, t := range ofxTag.FindAllStringSubmatch(m[1], -1) {
			k := strings.ToUpper(t[1])
			if _, dup := tags[k]; !dup {
				tags[k] = strings.TrimSpace(t[2])
			}
		}
		raw := tags["DTPOSTED"]
		if len(raw) < 8 {
			return nil, invalid("a transaction has no posted date")
		}
		date := raw[:4] + "-" + raw[4:6] + "-" + raw[6:8]
		if _, err := validDate(date); err != nil {
			return nil, invalid("bad date %q in the file", raw)
		}
		amt, err := strconv.ParseFloat(strings.ReplaceAll(tags["TRNAMT"], ",", "."), 64)
		if err != nil {
			return nil, invalid("bad amount %q in the file", tags["TRNAMT"])
		}
		desc := strings.TrimSpace(strings.Join(nonEmpty(tags["NAME"], tags["MEMO"]), " - "))
		if desc == "" {
			desc = orDefault(tags["TRNTYPE"], "Bank transaction")
		}
		out = append(out, StatementLine{Date: date, Description: unescapeXML(desc), Amount: paise(amt)})
	}
	if len(out) == 0 {
		return nil, invalid("no transactions found in the file")
	}
	return out, nil
}

func nonEmpty(s ...string) []string {
	var o []string
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			o = append(o, strings.TrimSpace(x))
		}
	}
	return o
}

func unescapeXML(s string) string {
	return strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'").Replace(s)
}

// ImportOFX imports an OFX/QFX statement into a bank account (duplicates are skipped like CSV).
func (r *Repository) ImportOFX(ctx context.Context, orgID, userID, accountID, data string) (ImportResult, error) {
	if _, err := r.bankAccount(ctx, orgID, accountID); err != nil {
		return ImportResult{}, err
	}
	lines, err := ParseOFX(data)
	if err != nil {
		return ImportResult{}, err
	}
	return r.importLines(ctx, orgID, userID, accountID, lines, "ofx")
}

var _ = fmt.Sprint
