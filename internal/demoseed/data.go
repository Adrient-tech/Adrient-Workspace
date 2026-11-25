package demoseed

import (
	"fmt"
	"math/rand"
	"strings"
)

// All names are invented combinations. Demo e-mail addresses use the reserved
// .example TLD so nothing can ever reach a real inbox.
const demoDomain = "demo.adrient.example"

var (
	firstNames = []string{"Aarav", "Ishita", "Kabir", "Meenal", "Nikhil", "Pooja", "Rohit", "Sanya", "Tarun", "Uma", "Vikram", "Waseem", "Yamini", "Zoya",
		"Anaya", "Bhavesh", "Chirag", "Divya", "Eshan", "Farhan", "Gauri", "Harsh", "Indira", "Jatin", "Kritika", "Lakshmi", "Manav", "Neha", "Omkar", "Pranav"}
	lastNames = []string{"Kulkarni", "Deshmukh", "Iyer", "Nambiar", "Chaudhary", "Bhatt", "Menon", "Rao", "Sethi", "Pillai", "Joshi", "Naik", "Gill",
		"Banerjee", "Trivedi", "Hegde", "Shetty", "Mahajan", "Reddy", "Thakur"}
	deptNames   = []string{"Engineering", "Sales", "Finance", "Operations", "People and Culture"}
	titlesByDep = map[string][]string{
		"Engineering":        {"Software Engineer", "Senior Software Engineer", "QA Engineer", "Engineering Manager", "DevOps Engineer"},
		"Sales":              {"Sales Executive", "Account Manager", "Sales Manager", "Inside Sales Associate"},
		"Finance":            {"Accountant", "Senior Accountant", "Finance Manager", "Accounts Executive"},
		"Operations":         {"Operations Executive", "Warehouse Supervisor", "Logistics Coordinator", "Operations Manager"},
		"People and Culture": {"HR Executive", "HR Manager", "Talent Partner", "Office Administrator"},
	}
	coPrefix = []string{"Kaveri", "Narmada", "Sahyadri", "Vindhya", "Godavari", "Malabar", "Konkan", "Aravalli", "Deccan", "Himalaya", "Chenab", "Tungabhadra",
		"Nilgiri", "Sundarban", "Brahmaputra", "Ganga", "Yamuna", "Satpura", "Western Ghats", "Coromandel"}
	coSuffix = []string{"Textiles", "Logistics", "Agro Foods", "Pharma Distributors", "Engineering Works", "Packaging", "Retail Mart", "Hardware Traders",
		"Print House", "Cold Chain", "Electricals", "Plastics", "Auto Components", "Stationers", "Dairy Products"}
	cities    = []string{"Pune", "Nashik", "Indore", "Coimbatore", "Surat", "Jaipur", "Kochi", "Nagpur", "Vadodara", "Mysuru"}
	prodKinds = []string{"Cotton Shirt", "Steel Bottle", "LED Bulb 9W", "Notebook A5", "Ball Pen Box", "Copier Paper Ream", "Packing Tape", "Bubble Wrap Roll",
		"Safety Gloves", "Water Filter Cartridge", "Spice Mix 500g", "Basmati Rice 5kg", "Tea Leaves 1kg", "Ghee Jar 1L", "Desk Lamp", "USB Cable 1m", "Power Strip",
		"Wall Clock", "Door Lock Set", "Paint Bucket 4L"}
	prodVariants = []string{"Classic", "Premium", "Eco", "Value Pack", "Industrial", "Home", "Office"}
	categories   = []string{"Apparel", "Kitchen", "Electricals", "Stationery", "Packaging", "Grocery", "Hardware"}
)

type rng struct{ *rand.Rand }

func newRNG() rng { return rng{rand.New(rand.NewSource(20261119))} }

func (r rng) pick(s []string) string { return s[r.Intn(len(s))] }

func (r rng) personName() (string, string) { return r.pick(firstNames), r.pick(lastNames) }

func (r rng) company() string { return r.pick(coPrefix) + " " + r.pick(coSuffix) }

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == ' ' || c == '-' || c == '.':
			b.WriteRune('.')
		}
	}
	return strings.Trim(b.String(), ".")
}

func demoEmail(first, last string) string {
	return fmt.Sprintf("%s.%s@%s", slug(first), slug(last), demoDomain)
}

func (r rng) phone() string {
	return fmt.Sprintf("+91 %d%d %05d %05d", 7+r.Intn(3), r.Intn(10), r.Intn(100000), r.Intn(100000))
}
