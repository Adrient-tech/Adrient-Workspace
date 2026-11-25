package pos

import (
	"crypto/rand"
	"math"
	"sort"
	"strings"
	"time"
)

// ---- manager PIN rules ----

// PinLockAfter is the number of wrong PINs before the terminal user is locked out.
const PinLockAfter = 5

// PinLockFor is how long that lock lasts.
const PinLockFor = 5 * time.Minute

// ValidatePIN checks a new PIN: 4 to 8 digits, not one repeated digit and not a
// straight run such as 1234 or 8765, which are the first things anyone tries.
func ValidatePIN(pin string) error {
	if len(pin) < 4 || len(pin) > 8 {
		return invalid("a PIN has 4 to 8 digits")
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return invalid("a PIN has digits only")
		}
	}
	same, up, down := true, true, true
	for i := 1; i < len(pin); i++ {
		if pin[i] != pin[0] {
			same = false
		}
		if pin[i] != pin[i-1]+1 {
			up = false
		}
		if pin[i] != pin[i-1]-1 {
			down = false
		}
	}
	if same || up || down {
		return invalid("that PIN is too easy to guess (repeated or in a row)")
	}
	return nil
}

// PinLocked reports whether a terminal user is locked out at now.
func PinLocked(lockedUntil *time.Time, now time.Time) bool {
	return lockedUntil != nil && now.Before(*lockedUntil)
}

// NextAttemptState returns the failure count and lock time after a wrong PIN.
// The fifth wrong try locks and resets the counter, so the lock expires cleanly.
func NextAttemptState(failed int, now time.Time) (newFailed int, lockedUntil *time.Time) {
	failed++
	if failed >= PinLockAfter {
		t := now.Add(PinLockFor)
		return 0, &t
	}
	return failed, nil
}

// ---- loyalty tiers and stamps ----

// Tier is a loyalty level reached by lifetime spend. Multiplier scales the
// points earned per sale (1 = normal).
type Tier struct {
	Name       string  `json:"name"`
	MinSpend   float64 `json:"min_spend"`
	Multiplier float64 `json:"multiplier"`
}

// TierFor returns the highest tier the lifetime spend has reached and whether
// there is one. Tiers need not be sorted.
func TierFor(tiers []Tier, spend float64) (Tier, bool) {
	sorted := append([]Tier(nil), tiers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinSpend < sorted[j].MinSpend })
	var best Tier
	found := false
	for _, t := range sorted {
		if spend+1e-9 >= t.MinSpend {
			best, found = t, true
		}
	}
	return best, found
}

// EarnPoints is the points for a sale total: floor(total x perUnit x tier multiplier).
func EarnPoints(total, perUnit float64, tier Tier, hasTier bool) float64 {
	if perUnit <= 0 || total <= 0 {
		return 0
	}
	m := 1.0
	if hasTier && tier.Multiplier > 0 {
		m = tier.Multiplier
	}
	return math.Floor(total*perUnit*m + 1e-9)
}

// StampRule is a "buy N, get a reward" card: every sale of at least MinSale
// adds one stamp, and each Goal stamps pay out RewardCredit as store credit.
type StampRule struct {
	Enabled      bool    `json:"enabled"`
	MinSale      float64 `json:"min_sale"`
	Goal         int     `json:"goal"`
	RewardCredit float64 `json:"reward_credit"`
}

// AddStamp applies one qualifying sale to a stamp count. It returns the new
// count and how many rewards were earned (a sale never pays out twice).
func AddStamp(rule StampRule, stamps int, saleTotal float64) (newStamps, rewards int) {
	if !rule.Enabled || rule.Goal < 1 || saleTotal+1e-9 < rule.MinSale {
		return stamps, 0
	}
	stamps++
	if stamps >= rule.Goal {
		return stamps - rule.Goal, 1
	}
	return stamps, 0
}

// ---- gift cards ----

const giftAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I

// NewGiftCode makes a code like "K7QD-M2XP-9WJA".
func NewGiftCode() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	var sb strings.Builder
	for i, x := range b {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(giftAlphabet[int(x)%len(giftAlphabet)])
	}
	return sb.String()
}

// NormalizeGiftCode upper-cases and trims a typed or scanned code.
func NormalizeGiftCode(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// GiftRef returns the gift card code when a payment ref names one ("gift:CODE").
func GiftRef(ref string) (string, bool) {
	r := strings.TrimSpace(ref)
	if len(r) > 5 && strings.EqualFold(r[:5], "gift:") {
		return NormalizeGiftCode(r[5:]), true
	}
	return "", false
}

// ---- discount approval ----

// SaleDiscountPercent is the effective discount share of a cart as a percent
// of its undiscounted value, counting line discounts and the order discount.
func SaleDiscountPercent(lines []CalcLine, kind string, value float64) float64 {
	var gross float64
	for _, l := range lines {
		gross += l.Qty * l.UnitPrice
	}
	if gross <= 0 {
		return 0
	}
	t := ComputeTotals(lines, kind, value)
	return math.Round((t.DiscountTotal/gross)*10000) / 100
}
