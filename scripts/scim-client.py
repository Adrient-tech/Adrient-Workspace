#!/usr/bin/env python3
"""A small SCIM 2.0 client that exercises an Adrient server the way Entra ID,
Okta and Google do: discovery, user create / filter / patch / put / delete and
group membership. Usage:

    scripts/scim-client.py --base http://localhost:8080/scim/v2 --token scim_xxx [--keep]

The token comes from Admin > Provisioning. Every check prints PASS or FAIL and
the exit code is the number of failures. The test users use the domain
scim-check.example and are removed at the end unless --keep is given.
"""
import argparse
import json
import sys
import urllib.error
import urllib.request
import uuid

failures = 0


def call(base, token, method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Authorization", "Bearer " + token)
    req.add_header("Content-Type", "application/scim+json")
    req.add_header("Accept", "application/scim+json")
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else {}), r.headers
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw), e.headers
        except ValueError:
            return e.code, {"raw": raw.decode(errors="replace")}, e.headers


def check(name, ok, detail=""):
    global failures
    print(("PASS  " if ok else "FAIL  ") + name + ("" if ok else "  -> " + str(detail)))
    if not ok:
        failures += 1


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--token", required=True)
    ap.add_argument("--keep", action="store_true")
    a = ap.parse_args()
    base, tok = a.base.rstrip("/"), a.token
    tag = uuid.uuid4().hex[:6]
    email = f"scim.{tag}@scim-check.example"

    c, b, _ = call(base, tok, "GET", "/ServiceProviderConfig")
    check("ServiceProviderConfig advertises PATCH and filter", c == 200 and b["patch"]["supported"] and b["filter"]["supported"], b)
    c, b, _ = call(base, tok, "GET", "/ResourceTypes")
    check("ResourceTypes lists User and Group", c == 200 and b["totalResults"] == 2, b)
    c, b, _ = call(base, tok, "GET", "/Schemas")
    check("Schemas answers", c == 200, b)
    c, _, _ = call(base, "scim_wrong", "GET", "/Users")
    check("a wrong token is refused with 401", c == 401)

    # Create (Entra-style body).
    c, u, h = call(base, tok, "POST", "/Users", {
        "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"], "userName": email, "externalId": "ext-" + tag,
        "name": {"givenName": "Scim", "familyName": "Check"}, "displayName": "Scim Check",
        "emails": [{"value": email, "type": "work", "primary": True}], "active": True, "title": "Tester"})
    check("create user returns 201", c == 201, (c, u))
    if c != 201:
        sys.exit(failures or 1)
    uid = u["id"]
    check("create returns Location and meta", bool(h.get("Location")) and u["meta"]["resourceType"] == "User", u)
    c, d, _ = call(base, tok, "POST", "/Users", {"userName": email})
    check("duplicate userName returns 409 uniqueness", c == 409 and d.get("scimType") == "uniqueness", (c, d))
    c, d, _ = call(base, tok, "GET", "/Users/" + uid)
    check("get user", c == 200 and d["userName"] == email and d["title"] == "Tester", d)
    c, d, _ = call(base, tok, "GET", '/Users?filter=' + urllib.request.quote(f'userName eq "{email.upper()}"'))
    check("filter userName eq (case-insensitive)", c == 200 and d["totalResults"] == 1 and d["Resources"][0]["id"] == uid, d)
    c, d, _ = call(base, tok, "GET", '/Users?filter=' + urllib.request.quote(f'externalId eq "ext-{tag}"'))
    check("filter externalId eq", c == 200 and d["totalResults"] == 1, d)
    c, d, _ = call(base, tok, "GET", "/Users?startIndex=1&count=1")
    check("pagination returns itemsPerPage <= count", c == 200 and d["itemsPerPage"] <= 1, d)

    # Patch: Entra uses string booleans and paths; Okta uses no path.
    c, d, _ = call(base, tok, "PATCH", "/Users/" + uid, {"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
        "Operations": [{"op": "Replace", "path": "active", "value": "False"}]})
    check("PATCH active=\"False\" (Entra) deactivates", c == 200 and d["active"] is False, (c, d))
    c, d, _ = call(base, tok, "PATCH", "/Users/" + uid, {"Operations": [{"op": "replace", "value": {"active": True, "title": "Lead Tester"}}]})
    check("PATCH without path (Okta) reactivates and updates title", c == 200 and d["active"] is True and d["title"] == "Lead Tester", (c, d))
    c, d, _ = call(base, tok, "PATCH", "/Users/" + uid, {"Operations": [{"op": "Replace", "path": "name.givenName", "value": "Skim"}]})
    check("PATCH name.givenName", c == 200 and d["name"]["givenName"] == "Skim", (c, d))
    c, d, _ = call(base, tok, "PATCH", "/Users/" + uid, {"Operations": [{"op": "move", "path": "x"}]})
    check("PATCH with an unknown op is 400", c == 400, (c, d))
    c, d, _ = call(base, tok, "PUT", "/Users/" + uid, {"userName": email, "displayName": "Scim Q Check", "active": True})
    check("PUT replaces the user", c == 200 and d["displayName"] == "Scim Q Check", (c, d))

    # Groups.
    c, g, _ = call(base, tok, "POST", "/Groups", {"displayName": "Scim check " + tag, "externalId": "gx-" + tag, "members": [{"value": uid}]})
    check("create group with a member", c == 201 and len(g["members"]) == 1, (c, g))
    gid = g.get("id", "")
    c, g, _ = call(base, tok, "PATCH", "/Groups/" + gid, {"Operations": [{"op": "remove", "path": f'members[value eq "{uid}"]'}]})
    check("PATCH remove member", c == 200 and len(g["members"]) == 0, (c, g))
    c, g, _ = call(base, tok, "PATCH", "/Groups/" + gid, {"Operations": [{"op": "add", "path": "members", "value": [{"value": uid}]}]})
    check("PATCH add member", c == 200 and len(g["members"]) == 1, (c, g))
    c, d, _ = call(base, tok, "GET", '/Groups?filter=' + urllib.request.quote(f'displayName eq "Scim check {tag}"'))
    check("filter group displayName", c == 200 and d["totalResults"] == 1, d)

    if not a.keep:
        c, _, _ = call(base, tok, "DELETE", "/Groups/" + gid)
        check("delete group returns 204", c == 204, c)
        c, _, _ = call(base, tok, "DELETE", "/Users/" + uid)
        check("delete user returns 204", c == 204, c)
        c, d, _ = call(base, tok, "GET", "/Users/" + uid)
        check("deleted user is gone (404)", c == 404, (c, d))
    print(f"\n{failures} failure(s)")
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
