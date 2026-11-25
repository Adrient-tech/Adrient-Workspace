package hr

import "context"

// LeaveSpan is an approved leave of one employee, for other modules (Shifts,
// Time tracking) that must not schedule or bill people on leave.
type LeaveSpan struct {
	EmployeeID string `json:"employee_id"`
	From       string `json:"from"`
	To         string `json:"to"`
	TypeName   string `json:"type_name"`
	HalfDay    string `json:"half_day"`
}

// ApprovedLeave lists approved leave overlapping from..to (YYYY-MM-DD) for the
// whole organisation. It is meant for server-side modules that apply their own
// access rules to what they show.
func (r *Repository) ApprovedLeave(ctx context.Context, orgID, from, to string) ([]LeaveSpan, error) {
	rows, err := r.pool.Query(ctx, `SELECT q.employee_id::text, to_char(q.from_date,'YYYY-MM-DD'), to_char(q.to_date,'YYYY-MM-DD'), t.name, q.half_day
		FROM adrient.hr_leave_requests q JOIN adrient.hr_leave_types t ON t.id=q.type_id
		WHERE q.org_id=$1 AND q.status='approved' AND q.to_date >= $2::date AND q.from_date <= $3::date ORDER BY q.from_date`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaveSpan{}
	for rows.Next() {
		var s LeaveSpan
		if err := rows.Scan(&s.EmployeeID, &s.From, &s.To, &s.TypeName, &s.HalfDay); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// EmployeeIDForUser returns the active employee record of a user ("" if none).
func (r *Repository) EmployeeIDForUser(ctx context.Context, orgID, userID string) string {
	var id string
	_ = r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.hr_employees WHERE org_id=$1 AND user_id=$2::uuid AND status<>'exited'`, orgID, userID).Scan(&id)
	return id
}
