package audit

import "context"

// Emit records a structured identity/lifecycle event (identity plan G1). The
// actor and org come from the Recorder's ActorResolver (the request context);
// detail may carry the optional keys "resource_type", "resource_id", "org_id",
// "actor_id", "actor_email", "ip", "ua", "status" which are lifted into the
// event columns (the rest stays in the JSON detail). Best-effort like Record.
func (r *Recorder) Emit(ctx context.Context, service, action string, detail map[string]any) {
	if r == nil {
		return
	}
	e := Event{Service: service, Action: action, Status: "ok", Method: "event " + service + "." + action}
	d := make(map[string]any, len(detail))
	for k, v := range detail {
		d[k] = v
	}
	lift := func(key string, dst *string) {
		if s, ok := d[key].(string); ok {
			*dst = s
			delete(d, key)
		}
	}
	lift("resource_type", &e.ResourceType)
	lift("resource_id", &e.ResourceID)
	lift("org_id", &e.OrgID)
	lift("actor_id", &e.ActorID)
	lift("actor_email", &e.ActorEmail)
	lift("ip", &e.IP)
	lift("ua", &e.UserAgent)
	var st string
	lift("status", &st)
	if st != "" {
		e.Status = st
	}
	e.Detail = d
	r.Record(ctx, e)
}
