package handlers

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/gateway/gateway/framework/logstore"
)

// =========================================================================
// SMART DLP VALIDATION ENGINE (Go Backend)
// Validates candidate matches mathematically so false positives are eliminated.
// =========================================================================

// 1. Luhn Mod 10 Check (Credit / Debit Cards, IMEI)
func luhnCheckGo(str string) bool {
	clean := strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", "")
	if len(clean) < 13 || len(clean) > 19 {
		return false
	}
	for _, ch := range clean {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	sum := 0
	alt := false
	for i := len(clean) - 1; i >= 0; i-- {
		d := int(clean[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// 2. Verhoeff Dihedral D5 Check (Aadhaar Card)
var (
	vDGo = [10][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6}, {3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8}, {5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2}, {7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4}, {9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	vPGo = [8][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, {1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2}, {8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0}, {4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5}, {7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
)

func verhoeffCheckGo(str string) bool {
	clean := strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", "")
	if len(clean) != 12 {
		return false
	}
	if clean[0] == '0' || clean[0] == '1' {
		return false
	}
	c := 0
	for i := 0; i < len(clean); i++ {
		d := int(clean[len(clean)-1-i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		c = vDGo[c][vPGo[i%8][d]]
	}
	return c == 0
}

// 3. IBAN Mod 97 (ISO 7064)
func ibanCheckGo(str string) bool {
	clean := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", ""))
	if len(clean) < 15 || len(clean) > 34 {
		return false
	}
	rearranged := clean[4:] + clean[:4]
	var numStr strings.Builder
	for _, ch := range rearranged {
		if ch >= 'A' && ch <= 'Z' {
			numStr.WriteString(strconv.Itoa(int(ch - 'A' + 10)))
		} else if ch >= '0' && ch <= '9' {
			numStr.WriteRune(ch)
		} else {
			return false
		}
	}
	s := numStr.String()
	rem := 0
	for len(s) > 0 {
		end := 7
		if len(s) < end {
			end = len(s)
		}
		chunk := strconv.Itoa(rem) + s[:end]
		s = s[end:]
		v, err := strconv.Atoi(chunk)
		if err != nil {
			return false
		}
		rem = v % 97
	}
	return rem == 1
}

// 4. PAN Check (Structure + Entity Type)
func panCheckGo(str string) bool {
	clean := strings.ToUpper(strings.TrimSpace(str))
	if len(clean) != 10 {
		return false
	}
	for i := 0; i < 5; i++ {
		if clean[i] < 'A' || clean[i] > 'Z' {
			return false
		}
	}
	for i := 5; i < 9; i++ {
		if clean[i] < '0' || clean[i] > '9' {
			return false
		}
	}
	if clean[9] < 'A' || clean[9] > 'Z' {
		return false
	}
	entity := clean[3]
	return strings.ContainsRune("PCHFATBLJG", rune(entity))
}

// 5. GSTIN Check (State + PAN)
func gstinCheckGo(str string) bool {
	clean := strings.ToUpper(strings.TrimSpace(str))
	if len(clean) != 15 {
		return false
	}
	st, err := strconv.Atoi(clean[:2])
	if err != nil || st < 1 || st > 38 {
		return false
	}
	return panCheckGo(clean[2:12])
}

// 6. US SSN Check
func ssnCheckGo(str string) bool {
	clean := strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", "")
	if len(clean) != 9 {
		return false
	}
	area, err1 := strconv.Atoi(clean[:3])
	group, err2 := strconv.Atoi(clean[3:5])
	serial, err3 := strconv.Atoi(clean[5:9])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	if area == 0 || area == 666 || area >= 900 {
		return false
	}
	if group == 0 || serial == 0 {
		return false
	}
	return true
}

// 7. Shannon Entropy Algorithm
func shannonEntropyGo(str string) float64 {
	if len(str) < 12 {
		return 0
	}
	freq := make(map[rune]float64)
	for _, r := range str {
		freq[r]++
	}
	length := float64(len(str))
	ent := 0.0
	for _, count := range freq {
		p := count / length
		ent -= p * math.Log2(p)
	}
	return ent
}

var (
	cardRegexGo   = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)
	aadhaarRegexGo = regexp.MustCompile(`\b[2-9]\d{3}[\s-]?\d{4}[\s-]?\d{4}\b`)
	panRegexGo     = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]\b`)
	gstinRegexGo   = regexp.MustCompile(`\b[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][0-9A-Z]Z[0-9A-Z]\b`)
	ibanRegexGo    = regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9 ]{11,34}\b`)
	ssnRegexGo     = regexp.MustCompile(`\b\d{3}[ -]?\d{2}[ -]?\d{4}\b`)
	awsRegexGo     = regexp.MustCompile(`\b(AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}\b`)
	jwtRegexGo     = regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{10,}\.eyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\b`)
)

// validateRuleMatchGo mathematically validates whether matched text is genuine sensitive data or a false positive
func validateRuleMatchGo(rule logstore.BrowserGuardRule, text string) bool {
	name := strings.ToLower(rule.Name)
	pat := strings.ToLower(rule.Pattern)
	desc := strings.ToLower(rule.Description)
	comb := name + " " + pat + " " + desc

	if strings.Contains(comb, "card") || strings.Contains(comb, "credit") || strings.Contains(comb, "debit") || strings.Contains(comb, "visa") || strings.Contains(comb, "mastercard") || strings.Contains(comb, "rupay") || strings.Contains(comb, "amex") || strings.Contains(pat, `\d{16}`) || strings.Contains(pat, `\d{4}`) {
		for _, m := range cardRegexGo.FindAllString(text, -1) {
			if luhnCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "aadhaar") || strings.Contains(comb, "uidai") || strings.Contains(pat, `\d{12}`) {
		for _, m := range aadhaarRegexGo.FindAllString(text, -1) {
			if verhoeffCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "pan") {
		for _, m := range panRegexGo.FindAllString(strings.ToUpper(text), -1) {
			if panCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "gst") || strings.Contains(comb, "gstin") {
		for _, m := range gstinRegexGo.FindAllString(strings.ToUpper(text), -1) {
			if gstinCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "iban") {
		for _, m := range ibanRegexGo.FindAllString(strings.ToUpper(text), -1) {
			if ibanCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "ssn") || strings.Contains(comb, "social security") {
		for _, m := range ssnRegexGo.FindAllString(text, -1) {
			if ssnCheckGo(m) {
				return true
			}
		}
		return false
	}

	if strings.Contains(comb, "aws") || strings.Contains(comb, "akia") {
		return awsRegexGo.MatchString(text)
	}

	if strings.Contains(comb, "jwt") || strings.Contains(comb, "bearer") {
		return jwtRegexGo.MatchString(text)
	}

	if strings.Contains(comb, "private key") || strings.Contains(comb, "rsa") {
		return strings.Contains(text, "-----BEGIN ") && strings.Contains(text, "PRIVATE KEY-----")
	}

	if strings.Contains(comb, "secret") || strings.Contains(comb, "entropy") || strings.Contains(comb, "password") {
		tokens := strings.Fields(text)
		for _, tok := range tokens {
			if len(tok) >= 16 && shannonEntropyGo(tok) > 4.2 {
				return true
			}
		}
		return false
	}

	// Standard rule: no specialized algorithm applies, trust the regex match
	return true
}
