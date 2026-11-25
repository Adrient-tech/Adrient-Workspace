package authz

import "testing"

func rule(id, control string, val any, st, sid string, prio int, mode string, cond *Condition) ControlRule {
	return ControlRule{ID: id, Control: control, Value: val, SubjectType: st, SubjectID: sid, Priority: prio, EffectMode: mode, Condition: cond}
}

func TestResolveControl(t *testing.T) {
	member := Subject{UserID: "u1", UserType: TypeMember, GroupIDs: []string{"g1"}, DeptIDs: []string{"d1"}, RoleKeys: []string{RoleMember}, IP: "10.1.2.3"}
	guest := Subject{UserID: "u2", UserType: TypeGuest, RoleKeys: []string{RoleGuest}, IP: "8.8.8.8"}
	ext := Subject{UserID: "u3", UserType: TypeExternal, RoleKeys: []string{RoleMember}}
	set, rs := ModeSet, ModeRestrictOnly
	cases := []struct {
		name  string
		key   string
		rules []ControlRule
		sub   Subject
		want  any
		src   string
	}{
		{"default member forwarding", "mail.external_forwarding", nil, member, true, "default"},
		{"default guest forwarding", "mail.external_forwarding", nil, guest, false, "default"},
		{"default external forwarding", "mail.external_forwarding", nil, ext, false, "default"},
		{"default guest download", "drive.guest_download", nil, guest, false, "default"},
		{"default enum", "drive.external_sharing", nil, member, "domains", "default"},
		{"default number", "mail.attachment_max_mb", nil, member, 25.0, "default"},
		{"everyone rule", "drive.external_sharing", []ControlRule{rule("a", "drive.external_sharing", "off", SubjectEveryone, "", 100, set, nil)}, member, "off", "rule"},
		{"user_type guest rule applies to guest", "drive.external_sharing", []ControlRule{rule("a", "drive.external_sharing", "off", SubjectUserType, TypeGuest, 100, set, nil)}, guest, "off", "rule"},
		{"user_type guest rule skips member", "drive.external_sharing", []ControlRule{rule("a", "drive.external_sharing", "off", SubjectUserType, TypeGuest, 100, set, nil)}, member, "domains", "default"},
		{"user beats group", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectGroup, "g1", 1, set, nil), rule("b", "meet.recording", "all", SubjectUser, "u1", 999, set, nil)}, member, "all", "rule"},
		{"group beats department", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectDepartment, "d1", 1, set, nil), rule("b", "meet.recording", "all", SubjectGroup, "g1", 50, set, nil)}, member, "all", "rule"},
		{"department beats user_type", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectUserType, TypeMember, 1, set, nil), rule("b", "meet.recording", "all", SubjectDepartment, "d1", 50, set, nil)}, member, "all", "rule"},
		{"user_type beats role", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectRole, RoleMember, 1, set, nil), rule("b", "meet.recording", "all", SubjectUserType, TypeMember, 50, set, nil)}, member, "all", "rule"},
		{"role beats everyone", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 1, set, nil), rule("b", "meet.recording", "all", SubjectRole, RoleMember, 50, set, nil)}, member, "all", "rule"},
		{"lower priority number wins in level", "meet.recording", []ControlRule{rule("a", "meet.recording", "all", SubjectEveryone, "", 50, set, nil), rule("b", "meet.recording", "off", SubjectEveryone, "", 10, set, nil)}, member, "off", "rule"},
		{"restrict_only beats specific allow", "drive.guest_download", []ControlRule{rule("a", "drive.guest_download", true, SubjectUser, "u2", 1, set, nil), rule("b", "drive.guest_download", false, SubjectEveryone, "", 100, rs, nil)}, guest, false, "rule"},
		{"restrict_only cannot loosen", "drive.guest_download", []ControlRule{rule("b", "drive.guest_download", true, SubjectEveryone, "", 100, rs, nil)}, guest, false, "default"},
		{"restrict_only enum narrows", "drive.external_sharing", []ControlRule{rule("a", "drive.external_sharing", "anyone", SubjectUser, "u1", 1, set, nil), rule("b", "drive.external_sharing", "domains", SubjectEveryone, "", 100, rs, nil)}, member, "domains", "rule"},
		{"restrict_only number min", "mail.attachment_max_mb", []ControlRule{rule("a", "mail.attachment_max_mb", 50.0, SubjectEveryone, "", 1, set, nil), rule("b", "mail.attachment_max_mb", 10.0, SubjectGroup, "g1", 100, rs, nil)}, member, 10.0, "rule"},
		{"ip condition met", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 100, set, &Condition{IPCIDRs: []string{"10.0.0.0/8"}})}, member, "off", "rule"},
		{"ip condition not met", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 100, set, &Condition{IPCIDRs: []string{"10.0.0.0/8"}})}, guest, "host", "default"},
		{"ip condition single address", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 100, set, &Condition{IPCIDRs: []string{"8.8.8.8"}})}, guest, "off", "rule"},
		{"user type condition", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 100, set, &Condition{UserTypes: []string{TypeGuest}})}, member, "host", "default"},
		{"department condition", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectEveryone, "", 100, set, &Condition{Departments: []string{"d1"}})}, member, "off", "rule"},
		{"other control ignored", "meet.recording", []ControlRule{rule("a", "meet.guests_allowed", false, SubjectEveryone, "", 100, set, nil)}, member, "host", "default"},
		{"unmatched group ignored", "meet.recording", []ControlRule{rule("a", "meet.recording", "off", SubjectGroup, "gX", 100, set, nil)}, member, "host", "default"},
	}
	for _, c := range cases {
		def, ok := ControlByKey(c.key)
		if !ok {
			t.Fatalf("%s: unknown control %s", c.name, c.key)
		}
		got := ResolveControl(def, c.rules, c.sub)
		if got.Value != c.want || got.Source != c.src {
			t.Errorf("%s: got %v (%s) want %v (%s)", c.name, got.Value, got.Source, c.want, c.src)
		}
	}
}

func TestControlCatalogAndNormalize(t *testing.T) {
	if _, ok := ControlByKey("nope.nothing"); ok {
		t.Fatal("unknown key accepted")
	}
	seen := map[string]bool{}
	for _, d := range Controls {
		if seen[d.Key] {
			t.Fatalf("duplicate %s", d.Key)
		}
		seen[d.Key] = true
		if _, err := d.Normalize(d.Default); err != nil {
			t.Errorf("%s default invalid: %v", d.Key, err)
		}
	}
	d, _ := ControlByKey("drive.external_sharing")
	if _, err := d.Normalize("everyone"); err == nil {
		t.Error("bad enum accepted")
	}
	b, _ := ControlByKey("meet.recording")
	if _, err := b.Normalize(true); err == nil {
		t.Error("bool accepted for enum")
	}
	n, _ := ControlByKey("mail.attachment_max_mb")
	if _, err := n.Normalize(500.0); err == nil {
		t.Error("out of range accepted")
	}
}

func TestPolicyUserTypeSubject(t *testing.T) {
	pol := []Policy{{AppID: "drive", SubjectType: SubjectUserType, SubjectID: TypeGuest, Effect: EffectDeny}}
	if Resolve(pol, "drive", Subject{UserID: "g", UserType: TypeGuest}).Allowed {
		t.Error("guest should be denied")
	}
	if !Resolve(pol, "drive", Subject{UserID: "m", UserType: TypeMember}).Allowed {
		t.Error("member must not be denied")
	}
	if !deniedBy(pol, "drive", Subject{UserType: TypeGuest}) || deniedBy(pol, "drive", Subject{UserType: TypeMember}) {
		t.Error("deniedBy mismatch")
	}
	ip := []Policy{{AppID: "drive", SubjectType: SubjectEveryone, Effect: EffectDeny, Condition: &Condition{IPCIDRs: []string{"1.2.3.0/24"}}}}
	if Resolve(ip, "drive", Subject{IP: "1.2.3.4"}).Allowed || !Resolve(ip, "drive", Subject{IP: "9.9.9.9"}).Allowed {
		t.Error("ip condition on policy")
	}
}
