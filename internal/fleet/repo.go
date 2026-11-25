package fleet

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/opsdoc"
	"github.com/Adrient-tech/Adrient-Workspace/internal/opshttp"
)

// Caller is the signed-in person; Admin = org admin.
type Caller struct {
	OrgID, UserID string
	Admin         bool
}

func (c Caller) needAdmin() error {
	if !c.Admin {
		return opshttp.Forbidden("only admins can change vehicles, service, reminders and costs")
	}
	return nil
}

// Settings are the fleet defaults.
type Settings struct {
	DueSoonDays     int      `json:"due_soon_days"`
	DueSoonMeter    float64  `json:"due_soon_meter"`
	MeterUnit       string   `json:"meter_unit"` // km | mi
	Currency        string   `json:"currency"`
	InspectionItems []string `json:"inspection_items"`
	ExpenseCategory string   `json:"expense_category_id"`
}

// DefaultInspection is the built-in pre-trip checklist.
var DefaultInspection = []string{"Tyres and wheels", "Brakes", "Lights and indicators", "Horn and wipers", "Mirrors and windscreen", "Fluid levels (oil, coolant)", "Seat belts", "Fire extinguisher and first-aid kit", "Body damage", "Documents in the vehicle"}

// Vehicle is one vehicle or piece of equipment.
type Vehicle struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	VIN           string  `json:"vin"`
	Plate         string  `json:"plate"`
	Make          string  `json:"make"`
	Model         string  `json:"model"`
	Year          int     `json:"year"`
	Type          string  `json:"type"`
	FuelType      string  `json:"fuel_type"`
	Status        string  `json:"status"` // active | maintenance | inactive | sold
	Group         string  `json:"group"`
	Meter         float64 `json:"meter"`
	AssigneeID    string  `json:"assignee_id"`
	AssigneeName  string  `json:"assignee_name,omitempty"`
	PurchaseDate  string  `json:"purchase_date"`
	PurchasePrice float64 `json:"purchase_price"`
	Notes         string  `json:"notes"`
	Archived      bool    `json:"archived"`
	// Derived.
	OpenIssues int `json:"open_issues,omitempty"`
	Overdue    int `json:"overdue,omitempty"`
	DueSoon    int `json:"due_soon,omitempty"`
}

// MeterReading is one odometer or hour-meter reading.
type MeterReading struct {
	ID        string  `json:"id"`
	VehicleID string  `json:"vehicle_id"`
	Date      string  `json:"date"`
	Value     float64 `json:"value"`
	Source    string  `json:"source"`
	Note      string  `json:"note"`
}

// Fuel is one fill-up.
type Fuel struct {
	ID        string  `json:"id"`
	VehicleID string  `json:"vehicle_id"`
	Date      string  `json:"date"`
	Quantity  float64 `json:"quantity"`
	Total     float64 `json:"total"`
	Meter     float64 `json:"meter"`
	Vendor    string  `json:"vendor"`
	FullTank  bool    `json:"full_tank"`
	Economy   float64 `json:"economy,omitempty"`
	ExpenseID string  `json:"expense_id"`
}

// Service is a maintenance or repair record.
type Service struct {
	ID          string   `json:"id"`
	VehicleID   string   `json:"vehicle_id"`
	Date        string   `json:"date"`
	Meter       float64  `json:"meter"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Cost        float64  `json:"cost"`
	ReminderIDs []string `json:"reminder_ids"`
	ExpenseID   string   `json:"expense_id"`
}

// Issue is a defect or problem to follow up.
type Issue struct {
	ID          string `json:"id"`
	VehicleID   string `json:"vehicle_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"` // low | medium | high
	Status      string `json:"status"`   // open | resolved | closed
	ReportedBy  string `json:"reported_by"`
	AssigneeID  string `json:"assignee_id"`
	DueDate     string `json:"due_date"`
	Resolution  string `json:"resolution"`
	Source      string `json:"source"` // manual | inspection
	CreatedAt   string `json:"created_at,omitempty"`
}

// InspectionItem is one checked line.
type InspectionItem struct {
	Label  string `json:"label"`
	Result string `json:"result"` // pass | fail | na
	Note   string `json:"note"`
}

// Inspection is a completed checklist.
type Inspection struct {
	ID        string           `json:"id"`
	VehicleID string           `json:"vehicle_id"`
	Date      string           `json:"date"`
	Meter     float64          `json:"meter"`
	By        string           `json:"by"`
	ByName    string           `json:"by_name,omitempty"`
	Items     []InspectionItem `json:"items"`
	Failed    int              `json:"failed"`
	IssueIDs  []string         `json:"issue_ids"`
}

// Repository is the Postgres store for Fleet.
type Repository struct {
	pool *pgxpool.Pool
	docs *opsdoc.Store
	cal  opshttp.CalendarWriter
	exp  opshttp.ExpenseWriter
	now  func() time.Time
}

// NewRepository builds the store; cal and exp may be nil.
func NewRepository(pool *pgxpool.Pool, cal opshttp.CalendarWriter, exp opshttp.ExpenseWriter) *Repository {
	return &Repository{pool: pool, docs: opsdoc.New(pool, "flt_docs"), cal: cal, exp: exp, now: time.Now}
}

func (r *Repository) today() string { return r.now().Format("2006-01-02") }

func idset[T any](f func(*T, string, string, string)) func(*T, opsdoc.Doc) {
	return func(v *T, d opsdoc.Doc) { f(v, d.ID, d.Ref, d.Day) }
}

// ---- settings ----

// GetSettings returns the defaults.
func (r *Repository) GetSettings(ctx context.Context, org string) (Settings, string, error) {
	s := Settings{DueSoonDays: 14, DueSoonMeter: 500, MeterUnit: "km", Currency: "INR", InspectionItems: DefaultInspection}
	l, err := r.docs.List(ctx, org, "settings", opsdoc.Filter{Limit: 1})
	if err != nil || len(l) == 0 {
		return s, "", err
	}
	if v, err := opsdoc.Decode[Settings](l[0], nil); err == nil {
		s = v
		if len(s.InspectionItems) == 0 {
			s.InspectionItems = DefaultInspection
		}
	}
	return s, l[0].ID, nil
}

// SaveSettings stores the defaults (admins).
func (r *Repository) SaveSettings(ctx context.Context, c Caller, s Settings) (Settings, error) {
	if err := c.needAdmin(); err != nil {
		return s, err
	}
	if s.DueSoonDays < 0 || s.DueSoonDays > 365 || s.DueSoonMeter < 0 || (s.MeterUnit != "km" && s.MeterUnit != "mi") {
		return s, opshttp.Invalid("due-soon thresholds or the meter unit are out of range")
	}
	var items []string
	for _, it := range s.InspectionItems {
		if it = strings.TrimSpace(it); it != "" && len(it) <= 120 {
			items = append(items, it)
		}
	}
	if len(items) == 0 || len(items) > 60 {
		return s, opshttp.Invalid("the inspection checklist needs between 1 and 60 items")
	}
	s.InspectionItems = items
	s.Currency = strings.ToUpper(strings.TrimSpace(s.Currency))
	if len(s.Currency) != 3 {
		s.Currency = "INR"
	}
	_, id, err := r.GetSettings(ctx, c.OrgID)
	if err != nil {
		return s, err
	}
	_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "settings", id, "", "", s)
	return s, err
}

// ---- vehicles ----

func (r *Repository) names(ctx context.Context, org string, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, display_name FROM adrient.users WHERE org_id=$1 AND id = ANY($2::uuid[])`, org, ids)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, n string
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

func vehOf(d opsdoc.Doc) (Vehicle, error) {
	return opsdoc.Decode(d, func(v *Vehicle, d opsdoc.Doc) { v.ID = d.ID })
}

// ListVehicles returns vehicles with their open issue and reminder counts.
func (r *Repository) ListVehicles(ctx context.Context, c Caller, status, q string, includeArchived bool) ([]Vehicle, error) {
	l, err := r.docs.List(ctx, c.OrgID, "vehicle", opsdoc.Filter{})
	if err != nil {
		return nil, err
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	issues, _ := r.ListIssues(ctx, c, "", "open")
	openBy := map[string]int{}
	for _, i := range issues {
		openBy[i.VehicleID]++
	}
	rems, _ := r.ListReminders(ctx, c, "")
	over, soon := map[string]int{}, map[string]int{}
	for _, x := range rems {
		switch x.Status {
		case "overdue":
			over[x.VehicleID]++
		case "due_soon":
			soon[x.VehicleID]++
		}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []Vehicle
	var ids []string
	for _, d := range l {
		v, err := vehOf(d)
		if err != nil || (v.Archived && !includeArchived) || (status != "" && v.Status != status) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Name+" "+v.Plate+" "+v.VIN+" "+v.Make+" "+v.Model+" "+v.Group), q) {
			continue
		}
		v.OpenIssues, v.Overdue, v.DueSoon = openBy[v.ID], over[v.ID], soon[v.ID]
		out = append(out, v)
		if v.AssigneeID != "" {
			ids = append(ids, v.AssigneeID)
		}
	}
	nm := r.names(ctx, c.OrgID, ids)
	for i := range out {
		out[i].AssigneeName = nm[out[i].AssigneeID]
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	_ = set
	if out == nil {
		out = []Vehicle{}
	}
	return out, nil
}

// GetVehicle returns one vehicle.
func (r *Repository) GetVehicle(ctx context.Context, c Caller, id string) (Vehicle, error) {
	d, err := r.docs.Get(ctx, c.OrgID, "vehicle", id)
	if err != nil {
		return Vehicle{}, err
	}
	v, err := vehOf(d)
	if v.AssigneeID != "" {
		v.AssigneeName = r.names(ctx, c.OrgID, []string{v.AssigneeID})[v.AssigneeID]
	}
	return v, err
}

// VehicleResult is a saved vehicle with warnings.
type VehicleResult struct {
	Vehicle  Vehicle  `json:"vehicle"`
	Warnings []string `json:"warnings"`
}

// SaveVehicle creates (id == "") or updates a vehicle (admins). The VIN is
// checked (shape is an error, the check digit a warning) and plates and VINs
// must be unique.
func (r *Repository) SaveVehicle(ctx context.Context, c Caller, id string, v Vehicle) (VehicleResult, error) {
	if err := c.needAdmin(); err != nil {
		return VehicleResult{}, err
	}
	v.Name, v.Plate = strings.TrimSpace(v.Name), strings.ToUpper(strings.TrimSpace(v.Plate))
	v.VIN = CleanVIN(v.VIN)
	if v.Name == "" || len(v.Name) > 120 || len(v.Notes) > 2000 {
		return VehicleResult{}, opshttp.Invalid("a vehicle needs a name of up to 120 characters")
	}
	switch v.Status {
	case "active", "maintenance", "inactive", "sold":
	case "":
		v.Status = "active"
	default:
		return VehicleResult{}, opshttp.Invalid("status must be active, maintenance, inactive or sold")
	}
	var warn []string
	if v.VIN != "" {
		shape, check := ValidVIN(v.VIN)
		if !shape {
			return VehicleResult{}, opshttp.Invalid("a VIN has 17 letters and digits (no I, O or Q)")
		}
		if !check {
			warn = append(warn, "The VIN check digit does not match. That is normal outside North America; check for typos.")
		}
		if v.Year == 0 {
			v.Year = ModelYear(v.VIN)
		}
	}
	if v.Year != 0 && (v.Year < 1950 || v.Year > r.now().Year()+2) || v.Meter < 0 || v.PurchasePrice < 0 {
		return VehicleResult{}, opshttp.Invalid("year, meter or price are out of range")
	}
	all, err := r.docs.List(ctx, c.OrgID, "vehicle", opsdoc.Filter{})
	if err != nil {
		return VehicleResult{}, err
	}
	for _, d := range all {
		o, err := vehOf(d)
		if err != nil || o.ID == id {
			continue
		}
		if v.Plate != "" && o.Plate == v.Plate {
			return VehicleResult{}, opshttp.Conflict("another vehicle (%s) already has the plate %s", o.Name, v.Plate)
		}
		if v.VIN != "" && o.VIN == v.VIN {
			return VehicleResult{}, opshttp.Conflict("another vehicle (%s) already has this VIN", o.Name)
		}
	}
	if id != "" {
		old, err := r.GetVehicle(ctx, c, id)
		if err != nil {
			return VehicleResult{}, err
		}
		if v.Meter < old.Meter {
			v.Meter = old.Meter // readings only go up through the meter log
		}
	}
	v.OpenIssues, v.Overdue, v.DueSoon, v.AssigneeName = 0, 0, 0, ""
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "vehicle", id, "", "", v)
	if err != nil {
		return VehicleResult{}, err
	}
	v.ID = d.ID
	if id == "" && v.Meter > 0 {
		_, _ = r.docs.Put(ctx, c.OrgID, c.UserID, "meter", "", v.ID, r.today(), MeterReading{VehicleID: v.ID, Value: v.Meter, Source: "manual", Note: "Initial reading"})
	}
	if id != "" {
		r.syncVehicleEvents(ctx, c, v)
	}
	return VehicleResult{Vehicle: v, Warnings: warn}, nil
}

// ---- meter, fuel, service ----

func (r *Repository) setMeter(ctx context.Context, c Caller, vehicleID string, value float64, date, source, note string, force bool) (Vehicle, []string, error) {
	v, err := r.GetVehicle(ctx, c, vehicleID)
	if err != nil {
		return v, nil, err
	}
	if value < 0 || value > 1e8 {
		return v, nil, opshttp.Invalid("the reading is out of range")
	}
	var warn []string
	if value < v.Meter {
		if !force {
			return v, nil, opshttp.Conflict("the reading %.0f is lower than the last one (%.0f). Check it, or confirm to record it anyway", value, v.Meter)
		}
		warn = append(warn, "Lower than the previous reading.")
	}
	if v.Meter > 0 && value-v.Meter > 5000 {
		warn = append(warn, fmt.Sprintf("That is %.0f more than the last reading. Check for a typo.", value-v.Meter))
	}
	if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "meter", "", vehicleID, date, MeterReading{VehicleID: vehicleID, Value: value, Source: source, Note: note}); err != nil {
		return v, warn, err
	}
	if value >= v.Meter {
		v.Meter = value
		v.OpenIssues, v.Overdue, v.DueSoon, v.AssigneeName = 0, 0, 0, ""
		if _, err := r.docs.Put(ctx, c.OrgID, c.UserID, "vehicle", vehicleID, "", "", v); err != nil {
			return v, warn, err
		}
	}
	return v, warn, nil
}

// AddMeter records a reading (any member).
func (r *Repository) AddMeter(ctx context.Context, c Caller, vehicleID string, value float64, date, note string, force bool) (Vehicle, []string, error) {
	if date == "" {
		date = r.today()
	}
	if !opshttp.ValidDate(date) {
		return Vehicle{}, nil, opshttp.Invalid("the date must be a date")
	}
	return r.setMeter(ctx, c, vehicleID, value, date, "manual", note, force)
}

// ListMeter returns a vehicle's readings, newest first.
func (r *Repository) ListMeter(ctx context.Context, c Caller, vehicleID string) ([]MeterReading, error) {
	l, err := r.docs.List(ctx, c.OrgID, "meter", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	out := opsdoc.DecodeAll(l, idset(func(v *MeterReading, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

// AddFuel records a fill-up (any member) and the meter reading with it.
func (r *Repository) AddFuel(ctx context.Context, c Caller, f Fuel) (Fuel, []string, error) {
	if !opshttp.ValidDate(f.Date) {
		return f, nil, opshttp.Invalid("the date must be a date")
	}
	if f.Quantity <= 0 || f.Quantity > 5000 || f.Total < 0 || f.Total > 1e7 || len(f.Vendor) > 120 {
		return f, nil, opshttp.Invalid("quantity must be above zero and the cost not negative")
	}
	var warn []string
	if f.Meter > 0 {
		var err error
		if _, warn, err = r.setMeter(ctx, c, f.VehicleID, f.Meter, f.Date, "fuel", "", false); err != nil {
			return f, nil, err
		}
	} else if _, err := r.GetVehicle(ctx, c, f.VehicleID); err != nil {
		return f, nil, err
	}
	f.ExpenseID, f.Economy = "", 0
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "fuel", "", f.VehicleID, f.Date, f)
	f.ID = d.ID
	return f, warn, err
}

// ListFuel returns a vehicle's fill-ups, newest first, with fuel economy
// between consecutive full-tank fill-ups.
func (r *Repository) ListFuel(ctx context.Context, c Caller, vehicleID string) ([]Fuel, error) {
	l, err := r.docs.List(ctx, c.OrgID, "fuel", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	out := opsdoc.DecodeAll(l, idset(func(v *Fuel, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Meter > out[j].Meter
	})
	for i := range out {
		if !out[i].FullTank || out[i].Meter <= 0 {
			continue
		}
		for j := i + 1; j < len(out); j++ {
			if out[j].FullTank && out[j].Meter > 0 && out[j].VehicleID == out[i].VehicleID {
				out[i].Economy = FuelEconomy(out[j].Meter, out[i].Meter, out[i].Quantity)
				break
			}
		}
	}
	return out, nil
}

// AddService records maintenance (admins) and completes the chosen reminders.
func (r *Repository) AddService(ctx context.Context, c Caller, s Service) (Service, error) {
	if err := c.needAdmin(); err != nil {
		return s, err
	}
	if !opshttp.ValidDate(s.Date) || s.Cost < 0 || s.Cost > 1e8 || strings.TrimSpace(s.Description) == "" || len(s.Description) > 2000 {
		return s, opshttp.Invalid("a service needs a date and a description, and the cost cannot be negative")
	}
	if s.Meter > 0 {
		if _, _, err := r.setMeter(ctx, c, s.VehicleID, s.Meter, s.Date, "service", "", false); err != nil {
			return s, err
		}
	} else if _, err := r.GetVehicle(ctx, c, s.VehicleID); err != nil {
		return s, err
	}
	s.ExpenseID = ""
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "service", "", s.VehicleID, s.Date, s)
	if err != nil {
		return s, err
	}
	s.ID = d.ID
	for _, id := range s.ReminderIDs {
		if _, err := r.CompleteReminder(ctx, c, id, s.Date, s.Meter); err != nil {
			return s, err
		}
	}
	return s, nil
}

// ListService returns a vehicle's service history.
func (r *Repository) ListService(ctx context.Context, c Caller, vehicleID string) ([]Service, error) {
	l, err := r.docs.List(ctx, c.OrgID, "service", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	return opsdoc.DecodeAll(l, idset(func(v *Service, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day })), nil
}

// ---- reminders and renewals, with Calendar ----

func (r *Repository) eventOwner(ctx context.Context, c Caller, vehicleID string) string {
	if v, err := r.GetVehicle(ctx, c, vehicleID); err == nil && v.AssigneeID != "" {
		return v.AssigneeID
	}
	return c.UserID
}

func (r *Repository) vehicleLabel(ctx context.Context, c Caller, id string) string {
	v, err := r.GetVehicle(ctx, c, id)
	if err != nil {
		return ""
	}
	if v.Plate != "" {
		return v.Name + " (" + v.Plate + ")"
	}
	return v.Name
}

// syncEvent keeps one all-day Calendar item on the due date of a reminder or renewal.
func (r *Repository) syncEvent(ctx context.Context, c Caller, vehicleID, title, due, eventID string) string {
	if r.cal == nil {
		return eventID
	}
	if due == "" {
		if eventID != "" {
			_ = r.cal.Delete(ctx, c.OrgID, eventID)
		}
		return ""
	}
	day, err := time.ParseInLocation("2006-01-02", due, time.Local)
	if err != nil {
		return eventID
	}
	ev := opshttp.CalEvent{Title: title + " – " + r.vehicleLabel(ctx, c, vehicleID), Description: "Fleet reminder", Start: day, End: day.AddDate(0, 0, 1), AllDay: true, Color: "#B45309"}
	if eventID != "" {
		if err := r.cal.Update(ctx, c.OrgID, eventID, ev); err == nil {
			return eventID
		}
	}
	id, err := r.cal.Create(ctx, c.OrgID, r.eventOwner(ctx, c, vehicleID), ev)
	if err != nil {
		return ""
	}
	return id
}

func (r *Repository) syncVehicleEvents(ctx context.Context, c Caller, v Vehicle) {
	rems, _ := r.ListReminders(ctx, c, v.ID)
	for _, x := range rems {
		if x.NextDate != "" {
			id := r.syncEvent(ctx, c, v.ID, x.Title, x.NextDate, x.EventID)
			if id != x.EventID {
				x.EventID = id
				r.putReminder(ctx, c, x)
			}
		}
	}
}

func (r *Repository) putReminder(ctx context.Context, c Caller, x Reminder) (Reminder, error) {
	id := x.ID
	x.Status, x.NextDate, x.NextMeter, x.DaysLeft, x.MeterLeft = "", "", nil, nil, nil
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "reminder", id, x.VehicleID, "", x)
	x.ID = d.ID
	return x, err
}

func (r *Repository) derive(x Reminder, meter float64, set Settings) Reminder {
	st := x.StateAt(meter, r.today(), set.DueSoonDays, set.DueSoonMeter)
	x.Status, x.NextDate, x.NextMeter, x.DaysLeft, x.MeterLeft = st.Status, st.NextDate, st.NextMeter, st.DaysLeft, st.MeterLeft
	return x
}

// ListReminders returns reminders with their derived state (all vehicles when vehicleID is empty).
func (r *Repository) ListReminders(ctx context.Context, c Caller, vehicleID string) ([]Reminder, error) {
	l, err := r.docs.List(ctx, c.OrgID, "reminder", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	meters := map[string]float64{}
	vl, _ := r.docs.List(ctx, c.OrgID, "vehicle", opsdoc.Filter{})
	for _, d := range vl {
		if v, err := vehOf(d); err == nil {
			meters[v.ID] = v.Meter
		}
	}
	out := []Reminder{}
	for _, x := range opsdoc.DecodeAll(l, idset(func(v *Reminder, id, ref, day string) { v.ID, v.VehicleID = id, ref })) {
		out = append(out, r.derive(x, meters[x.VehicleID], set))
	}
	sort.SliceStable(out, func(i, j int) bool {
		rank := map[string]int{"overdue": 0, "due_soon": 1, "snoozed": 2, "upcoming": 3}
		if rank[out[i].Status] != rank[out[j].Status] {
			return rank[out[i].Status] < rank[out[j].Status]
		}
		return out[i].NextDate < out[j].NextDate
	})
	return out, nil
}

// SaveReminder creates or updates a reminder (admins).
func (r *Repository) SaveReminder(ctx context.Context, c Caller, id string, x Reminder) (Reminder, error) {
	if err := c.needAdmin(); err != nil {
		return x, err
	}
	x.Title = strings.TrimSpace(x.Title)
	v, err := r.GetVehicle(ctx, c, x.VehicleID)
	if err != nil {
		return x, opshttp.Invalid("choose a vehicle")
	}
	if x.Title == "" || len(x.Title) > 120 || (x.Interval.Every <= 0 && x.MeterInterval <= 0) {
		return x, opshttp.Invalid("a reminder needs a title and a time or meter interval")
	}
	if x.Interval.Every > 0 {
		switch x.Interval.Unit {
		case "days", "weeks", "months", "years":
		default:
			return x, opshttp.Invalid("the interval unit must be days, weeks, months or years")
		}
	}
	if x.Interval.Every < 0 || x.Interval.Every > 1000 || x.MeterInterval < 0 || x.MeterInterval > 1e7 {
		return x, opshttp.Invalid("interval out of range")
	}
	if x.LastDate == "" && x.StartDate == "" {
		x.StartDate, x.StartMeter = r.today(), v.Meter
	}
	if (x.StartDate != "" && !opshttp.ValidDate(x.StartDate)) || (x.LastDate != "" && !opshttp.ValidDate(x.LastDate)) {
		return x, opshttp.Invalid("dates must be valid")
	}
	if id != "" {
		old, err := r.docs.Get(ctx, c.OrgID, "reminder", id)
		if err != nil {
			return x, err
		}
		prev, _ := opsdoc.Decode[Reminder](old, nil)
		x.History, x.EventID = prev.History, prev.EventID
	}
	x.ID = id
	saved, err := r.putReminder(ctx, c, x)
	if err != nil {
		return saved, err
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	saved = r.derive(saved, v.Meter, set)
	if ev := r.syncEvent(ctx, c, saved.VehicleID, saved.Title, saved.NextDate, saved.EventID); ev != saved.EventID {
		saved.EventID = ev
		saved, _ = r.putReminder(ctx, c, saved)
		saved = r.derive(saved, v.Meter, set)
	}
	return saved, nil
}

// CompleteReminder records a completion and moves the Calendar item to the next due date.
func (r *Repository) CompleteReminder(ctx context.Context, c Caller, id, date string, meter float64) (Reminder, error) {
	if err := c.needAdmin(); err != nil {
		return Reminder{}, err
	}
	d, err := r.docs.Get(ctx, c.OrgID, "reminder", id)
	if err != nil {
		return Reminder{}, err
	}
	x, _ := opsdoc.Decode(d, idset(func(v *Reminder, id, ref, day string) { v.ID, v.VehicleID = id, ref }))
	if date == "" {
		date = r.today()
	}
	if !opshttp.ValidDate(date) {
		return x, opshttp.Invalid("the date must be a date")
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	if meter <= 0 {
		if v, err := r.GetVehicle(ctx, c, x.VehicleID); err == nil {
			meter = v.Meter
		}
	}
	x = x.Complete(date, meter, r.today(), set.DueSoonDays, set.DueSoonMeter)
	nd, _ := x.NextDue()
	x.EventID = r.syncEvent(ctx, c, x.VehicleID, x.Title, nd, x.EventID)
	saved, err := r.putReminder(ctx, c, x)
	if err != nil {
		return saved, err
	}
	m := meter
	return r.derive(saved, m, set), nil
}

// SnoozeReminder hides a reminder's alerts until a date.
func (r *Repository) SnoozeReminder(ctx context.Context, c Caller, id, until string) (Reminder, error) {
	if err := c.needAdmin(); err != nil {
		return Reminder{}, err
	}
	if !opshttp.ValidDate(until) {
		return Reminder{}, opshttp.Invalid("choose a date to snooze until")
	}
	d, err := r.docs.Get(ctx, c.OrgID, "reminder", id)
	if err != nil {
		return Reminder{}, err
	}
	x, _ := opsdoc.Decode(d, idset(func(v *Reminder, id, ref, day string) { v.ID, v.VehicleID = id, ref }))
	x.SnoozedUntil = until
	return r.putReminder(ctx, c, x)
}

// DeleteReminder removes a reminder and its Calendar item.
func (r *Repository) DeleteReminder(ctx context.Context, c Caller, id string) error {
	if err := c.needAdmin(); err != nil {
		return err
	}
	d, err := r.docs.Get(ctx, c.OrgID, "reminder", id)
	if err != nil {
		return err
	}
	x, _ := opsdoc.Decode[Reminder](d, nil)
	if x.EventID != "" && r.cal != nil {
		_ = r.cal.Delete(ctx, c.OrgID, x.EventID)
	}
	return r.docs.Delete(ctx, c.OrgID, "reminder", id)
}

// ListRenewals returns renewals with their state.
func (r *Repository) ListRenewals(ctx context.Context, c Caller, vehicleID string) ([]Renewal, error) {
	l, err := r.docs.List(ctx, c.OrgID, "renewal", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	out := []Renewal{}
	for _, x := range opsdoc.DecodeAll(l, idset(func(v *Renewal, id, ref, day string) { v.ID, v.VehicleID = id, ref })) {
		x.Status, x.DaysLeft = x.StateAt(r.today(), set.DueSoonDays)
		out = append(out, x)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DueDate < out[j].DueDate })
	return out, nil
}

func (r *Repository) putRenewal(ctx context.Context, c Caller, x Renewal) (Renewal, error) {
	id := x.ID
	x.Status, x.DaysLeft = "", nil
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "renewal", id, x.VehicleID, "", x)
	x.ID = d.ID
	return x, err
}

// SaveRenewal creates or updates a renewal (admins).
func (r *Repository) SaveRenewal(ctx context.Context, c Caller, id string, x Renewal) (Renewal, error) {
	if err := c.needAdmin(); err != nil {
		return x, err
	}
	x.Title = strings.TrimSpace(x.Title)
	if _, err := r.GetVehicle(ctx, c, x.VehicleID); err != nil {
		return x, opshttp.Invalid("choose a vehicle")
	}
	if x.Title == "" || !opshttp.ValidDate(x.DueDate) || x.Cost < 0 || x.Repeat.Every < 0 || x.Repeat.Every > 100 {
		return x, opshttp.Invalid("a renewal needs a title and a due date")
	}
	if x.Repeat.Every > 0 && x.Repeat.Unit != "days" && x.Repeat.Unit != "weeks" && x.Repeat.Unit != "months" && x.Repeat.Unit != "years" {
		return x, opshttp.Invalid("the repeat unit must be days, weeks, months or years")
	}
	if id != "" {
		old, err := r.docs.Get(ctx, c.OrgID, "renewal", id)
		if err != nil {
			return x, err
		}
		prev, _ := opsdoc.Decode[Renewal](old, nil)
		x.History, x.EventID = prev.History, prev.EventID
	}
	x.ID, x.Done = id, false
	x.EventID = r.syncEvent(ctx, c, x.VehicleID, x.Title, x.DueDate, x.EventID)
	saved, err := r.putRenewal(ctx, c, x)
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	saved.Status, saved.DaysLeft = saved.StateAt(r.today(), set.DueSoonDays)
	return saved, err
}

// CompleteRenewal records the renewal and rolls the due date (and Calendar item) forward.
func (r *Repository) CompleteRenewal(ctx context.Context, c Caller, id, date string, cost float64, nextDue string) (Renewal, error) {
	if err := c.needAdmin(); err != nil {
		return Renewal{}, err
	}
	d, err := r.docs.Get(ctx, c.OrgID, "renewal", id)
	if err != nil {
		return Renewal{}, err
	}
	x, _ := opsdoc.Decode(d, idset(func(v *Renewal, id, ref, day string) { v.ID, v.VehicleID = id, ref }))
	if date == "" {
		date = r.today()
	}
	if !opshttp.ValidDate(date) || (nextDue != "" && !opshttp.ValidDate(nextDue)) || cost < 0 {
		return x, opshttp.Invalid("dates must be valid and the cost not negative")
	}
	x = x.Complete(date, cost, nextDue)
	if x.Done {
		x.EventID = r.syncEvent(ctx, c, x.VehicleID, x.Title, "", x.EventID)
	} else {
		x.EventID = r.syncEvent(ctx, c, x.VehicleID, x.Title, x.DueDate, x.EventID)
	}
	saved, err := r.putRenewal(ctx, c, x)
	set, _, _ := r.GetSettings(ctx, c.OrgID)
	saved.Status, saved.DaysLeft = saved.StateAt(r.today(), set.DueSoonDays)
	return saved, err
}

// ---- issues and inspections ----

// ListIssues returns issues (all vehicles when vehicleID is empty, any status when status is empty).
func (r *Repository) ListIssues(ctx context.Context, c Caller, vehicleID, status string) ([]Issue, error) {
	l, err := r.docs.List(ctx, c.OrgID, "issue", opsdoc.Filter{Ref: vehicleID})
	if err != nil {
		return nil, err
	}
	out := []Issue{}
	for _, i := range opsdoc.DecodeAll(l, func(v *Issue, d opsdoc.Doc) {
		v.ID, v.VehicleID, v.CreatedAt = d.ID, d.Ref, d.CreatedAt.Format("2006-01-02")
	}) {
		if status == "" || i.Status == status {
			out = append(out, i)
		}
	}
	return out, nil
}

// SaveIssue reports (id == "", any member) or updates (admins) an issue.
func (r *Repository) SaveIssue(ctx context.Context, c Caller, id string, in Issue) (Issue, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 160 || len(in.Description) > 4000 {
		return in, opshttp.Invalid("an issue needs a title of up to 160 characters")
	}
	if in.Priority != "low" && in.Priority != "high" {
		in.Priority = "medium"
	}
	if in.DueDate != "" && !opshttp.ValidDate(in.DueDate) {
		return in, opshttp.Invalid("the due date must be a date")
	}
	if _, err := r.GetVehicle(ctx, c, in.VehicleID); err != nil {
		return in, opshttp.Invalid("choose a vehicle")
	}
	if id == "" {
		in.Status, in.ReportedBy, in.Resolution = "open", c.UserID, ""
		if in.Source == "" {
			in.Source = "manual"
		}
	} else {
		if err := c.needAdmin(); err != nil {
			return in, err
		}
		d, err := r.docs.Get(ctx, c.OrgID, "issue", id)
		if err != nil {
			return in, err
		}
		old, _ := opsdoc.Decode[Issue](d, nil)
		in.ReportedBy, in.Source = old.ReportedBy, old.Source
		if in.Status != "open" && in.Status != "resolved" && in.Status != "closed" {
			in.Status = old.Status
		}
	}
	in.CreatedAt = ""
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "issue", id, in.VehicleID, "", in)
	in.ID = d.ID
	return in, err
}

// DefaultChecklist returns the inspection checklist.
func (r *Repository) DefaultChecklist(ctx context.Context, org string) []string {
	s, _, _ := r.GetSettings(ctx, org)
	return s.InspectionItems
}

// SubmitInspection stores a completed checklist (any member); every failed
// item opens an issue and the meter reading, if any, is recorded.
func (r *Repository) SubmitInspection(ctx context.Context, c Caller, in Inspection) (Inspection, error) {
	v, err := r.GetVehicle(ctx, c, in.VehicleID)
	if err != nil {
		return in, err
	}
	if len(in.Items) == 0 || len(in.Items) > 100 {
		return in, opshttp.Invalid("an inspection needs at least one checked item")
	}
	if in.Date == "" {
		in.Date = r.today()
	}
	if !opshttp.ValidDate(in.Date) {
		return in, opshttp.Invalid("the date must be a date")
	}
	if in.Meter > 0 {
		if _, _, err := r.setMeter(ctx, c, in.VehicleID, in.Meter, in.Date, "inspection", "", false); err != nil {
			return in, err
		}
	}
	in.By, in.Failed, in.IssueIDs = c.UserID, 0, []string{}
	for i := range in.Items {
		it := &in.Items[i]
		it.Label = strings.TrimSpace(it.Label)
		if it.Label == "" || (it.Result != "pass" && it.Result != "fail" && it.Result != "na") || len(it.Note) > 500 {
			return in, opshttp.Invalid("every item needs a label and a result of pass, fail or na")
		}
		if it.Result == "fail" {
			in.Failed++
		}
	}
	d, err := r.docs.Put(ctx, c.OrgID, c.UserID, "inspection", "", in.VehicleID, in.Date, in)
	if err != nil {
		return in, err
	}
	in.ID = d.ID
	for _, it := range in.Items {
		if it.Result != "fail" {
			continue
		}
		i, err := r.SaveIssue(ctx, c, "", Issue{VehicleID: v.ID, Title: "Inspection failed: " + it.Label, Description: it.Note, Priority: "high", Source: "inspection"})
		if err != nil {
			return in, err
		}
		in.IssueIDs = append(in.IssueIDs, i.ID)
	}
	if len(in.IssueIDs) > 0 {
		_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "inspection", in.ID, in.VehicleID, in.Date, in)
	}
	return in, err
}

// ListInspections returns inspections, newest first.
func (r *Repository) ListInspections(ctx context.Context, c Caller, vehicleID string) ([]Inspection, error) {
	l, err := r.docs.List(ctx, c.OrgID, "inspection", opsdoc.Filter{Ref: vehicleID, Limit: 200})
	if err != nil {
		return nil, err
	}
	out := opsdoc.DecodeAll(l, idset(func(v *Inspection, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
	ids := []string{}
	for _, i := range out {
		ids = append(ids, i.By)
	}
	nm := r.names(ctx, c.OrgID, ids)
	for i := range out {
		out[i].ByName = nm[out[i].By]
	}
	return out, nil
}

// ---- Accounting ----

// ExpenseCategories lists Accounting expense categories for posting.
func (r *Repository) ExpenseCategories(ctx context.Context, c Caller) ([]opshttp.ExpenseAccount, error) {
	if r.exp == nil {
		return []opshttp.ExpenseAccount{}, nil
	}
	return r.exp.Categories(ctx, c.OrgID)
}

// PostExpense records a fuel or service cost as a bill in Accounting (admins).
// Posting twice is refused.
func (r *Repository) PostExpense(ctx context.Context, c Caller, kind, id, categoryID string) (string, error) {
	if err := c.needAdmin(); err != nil {
		return "", err
	}
	if r.exp == nil {
		return "", opshttp.Invalid("Accounting is not available")
	}
	if kind != "fuel" && kind != "service" {
		return "", opshttp.Invalid("kind must be fuel or service")
	}
	d, err := r.docs.Get(ctx, c.OrgID, kind, id)
	if err != nil {
		return "", err
	}
	var date, vendor, note, existing, vehicle string
	var amount float64
	if kind == "fuel" {
		f, _ := opsdoc.Decode(d, idset(func(v *Fuel, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
		date, vendor, amount, existing, vehicle = f.Date, f.Vendor, f.Total, f.ExpenseID, f.VehicleID
		note = fmt.Sprintf("Fuel %.1f", f.Quantity)
	} else {
		s, _ := opsdoc.Decode(d, idset(func(v *Service, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
		date, vendor, amount, existing, vehicle = s.Date, s.Vendor, s.Cost, s.ExpenseID, s.VehicleID
		note = "Service: " + s.Description
	}
	if existing != "" {
		return existing, opshttp.Conflict("this cost is already in Accounting")
	}
	if amount <= 0 {
		return "", opshttp.Invalid("there is no cost to post")
	}
	if categoryID == "" {
		set, _, _ := r.GetSettings(ctx, c.OrgID)
		categoryID = set.ExpenseCategory
	}
	if !opshttp.IsUUID(categoryID) {
		return "", opshttp.Invalid("choose an Accounting expense category")
	}
	exp, err := r.exp.Record(ctx, c.OrgID, c.UserID, opshttp.ExpenseDraft{Date: date, Vendor: vendor, CategoryID: categoryID, Amount: amount,
		Notes: "Fleet: " + r.vehicleLabel(ctx, c, vehicle) + " – " + note})
	if err != nil {
		return "", err
	}
	if kind == "fuel" {
		f, _ := opsdoc.Decode(d, idset(func(v *Fuel, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
		f.ExpenseID = exp
		_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "fuel", id, f.VehicleID, f.Date, f)
	} else {
		s, _ := opsdoc.Decode(d, idset(func(v *Service, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day }))
		s.ExpenseID = exp
		_, err = r.docs.Put(ctx, c.OrgID, c.UserID, "service", id, s.VehicleID, s.Date, s)
	}
	return exp, err
}

// ---- dashboard and costs ----

// MonthCost is the fuel and service spend of one month.
type MonthCost struct {
	Month   string  `json:"month"`
	Fuel    float64 `json:"fuel"`
	Service float64 `json:"service"`
}

// Dashboard is the fleet overview.
type Dashboard struct {
	Vehicles      int            `json:"vehicles"`
	ByStatus      map[string]int `json:"by_status"`
	OpenIssues    int            `json:"open_issues"`
	Overdue       []Reminder     `json:"overdue_reminders"`
	DueSoon       []Reminder     `json:"due_soon_reminders"`
	Renewals      []Renewal      `json:"renewals_due"`
	Months        []MonthCost    `json:"months"`
	CostPerUnit   float64        `json:"cost_per_distance"`
	FailedLast30d int            `json:"failed_inspections_30d"`
}

// Dashboard summarises the fleet.
func (r *Repository) Dashboard(ctx context.Context, c Caller) (Dashboard, error) {
	d := Dashboard{ByStatus: map[string]int{}}
	vs, err := r.ListVehicles(ctx, c, "", "", false)
	if err != nil {
		return d, err
	}
	d.Vehicles = len(vs)
	for _, v := range vs {
		d.ByStatus[v.Status]++
		d.OpenIssues += v.OpenIssues
	}
	rems, _ := r.ListReminders(ctx, c, "")
	for _, x := range rems {
		switch x.Status {
		case "overdue":
			d.Overdue = append(d.Overdue, x)
		case "due_soon":
			d.DueSoon = append(d.DueSoon, x)
		}
	}
	ren, _ := r.ListRenewals(ctx, c, "")
	for _, x := range ren {
		if x.Status == "overdue" || x.Status == "due_soon" {
			d.Renewals = append(d.Renewals, x)
		}
	}
	months := map[string]*MonthCost{}
	var order []string
	for i := 5; i >= 0; i-- {
		k := r.now().AddDate(0, -i, 0).Format("2006-01")
		months[k] = &MonthCost{Month: k}
		order = append(order, k)
	}
	from := order[0] + "-01"
	fl, _ := r.docs.List(ctx, c.OrgID, "fuel", opsdoc.Filter{From: from})
	var spend float64
	for _, f := range opsdoc.DecodeAll(fl, idset(func(v *Fuel, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day })) {
		if m := months[f.Date[:7]]; m != nil {
			m.Fuel += f.Total
			spend += f.Total
		}
	}
	sl, _ := r.docs.List(ctx, c.OrgID, "service", opsdoc.Filter{From: from})
	for _, s := range opsdoc.DecodeAll(sl, idset(func(v *Service, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day })) {
		if m := months[s.Date[:7]]; m != nil {
			m.Service += s.Cost
			spend += s.Cost
		}
	}
	for _, k := range order {
		m := months[k]
		m.Fuel, m.Service = opshttp.Round2(m.Fuel), opshttp.Round2(m.Service)
		d.Months = append(d.Months, *m)
	}
	var distance float64
	ml, _ := r.docs.List(ctx, c.OrgID, "meter", opsdoc.Filter{From: from})
	lo, hi := map[string]float64{}, map[string]float64{}
	for _, m := range opsdoc.DecodeAll(ml, idset(func(v *MeterReading, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day })) {
		if v, ok := lo[m.VehicleID]; !ok || m.Value < v {
			lo[m.VehicleID] = m.Value
		}
		if m.Value > hi[m.VehicleID] {
			hi[m.VehicleID] = m.Value
		}
	}
	for id := range hi {
		distance += hi[id] - lo[id]
	}
	d.CostPerUnit = CostPerDistance(spend, distance)
	il, _ := r.docs.List(ctx, c.OrgID, "inspection", opsdoc.Filter{From: opshttp.AddDays(r.today(), -30)})
	for _, i := range opsdoc.DecodeAll(il, idset(func(v *Inspection, id, ref, day string) { v.ID, v.VehicleID, v.Date = id, ref, day })) {
		if i.Failed > 0 {
			d.FailedLast30d++
		}
	}
	return d, nil
}

// VehicleCosts are the lifetime costs of one vehicle.
type VehicleCosts struct {
	Fuel, Service, Total float64
	Distance             float64
	PerDistance          float64
}

// Costs sums fuel and service spend and the distance driven.
func (r *Repository) Costs(ctx context.Context, c Caller, vehicleID string) (VehicleCosts, error) {
	var vc VehicleCosts
	fuel, err := r.ListFuel(ctx, c, vehicleID)
	if err != nil {
		return vc, err
	}
	for _, f := range fuel {
		vc.Fuel += f.Total
	}
	sv, err := r.ListService(ctx, c, vehicleID)
	if err != nil {
		return vc, err
	}
	for _, s := range sv {
		vc.Service += s.Cost
	}
	ms, _ := r.ListMeter(ctx, c, vehicleID)
	if len(ms) > 1 {
		lo, hi := ms[0].Value, ms[0].Value
		for _, m := range ms {
			lo, hi = min(lo, m.Value), max(hi, m.Value)
		}
		vc.Distance = hi - lo
	}
	vc.Fuel, vc.Service = opshttp.Round2(vc.Fuel), opshttp.Round2(vc.Service)
	vc.Total = opshttp.Round2(vc.Fuel + vc.Service)
	vc.PerDistance = CostPerDistance(vc.Total, vc.Distance)
	return vc, nil
}
