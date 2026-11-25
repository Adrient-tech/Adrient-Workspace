package zitadelmgmt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	bad := []IdPInput{
		{Kind: KindEntra, Name: "x"},
		{Kind: KindGoogle, Name: "x", ClientID: "a"},
		{Kind: KindOIDC, Name: "x", ClientID: "a", ClientSecret: "b", Issuer: "nope"},
		{Kind: KindSAML, Name: "x"},
		{Kind: "ftp", Name: "x"},
		{Kind: KindGoogle, ClientID: "a", ClientSecret: "b"},
	}
	for i, in := range bad {
		if in.Validate() == nil {
			t.Errorf("case %d should be invalid", i)
		}
	}
	if (IdPInput{Kind: KindOIDC, Name: "ok", ClientID: "a", ClientSecret: "b", Issuer: "https://idp.test"}).Validate() != nil {
		t.Error("valid oidc rejected")
	}
}

func TestNilClient(t *testing.T) {
	var c *Client
	if _, err := c.CreateHumanUser(context.Background(), NewUser{Email: "a@b.test"}); err != ErrNotConfigured {
		t.Fatalf("got %v", err)
	}
	if New("", "x") != nil || New("http://x", "") != nil {
		t.Fatal("New must return nil when unconfigured")
	}
}

func TestCreateIdPRequests(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"id":"idp1"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "tok")
	id, err := c.CreateIdP(context.Background(), IdPInput{Kind: KindEntra, Name: "Contoso", ClientID: "a", ClientSecret: "b", TenantID: "t-123"})
	if err != nil || id != "idp1" {
		t.Fatalf("%v %q", err, id)
	}
	want := "GET /admin/v1/policies/login|PUT /admin/v1/policies/login|POST /admin/v1/idps/azure|POST /admin/v1/policies/login/idps"
	if got := strings.Join(paths, "|"); got != want {
		t.Fatalf("calls %q want %q", got, want)
	}
}

// Live check against a local Zitadel (ZITADEL_URL, ZITADEL_TOKEN): every kind
// is created, listed and removed again.
func TestLiveIdPs(t *testing.T) {
	c := New(os.Getenv("ZITADEL_URL"), os.Getenv("ZITADEL_TOKEN"))
	if c == nil {
		t.Skip("ZITADEL_URL/ZITADEL_TOKEN not set")
	}
	ctx := context.Background()
	for _, in := range []IdPInput{
		{Kind: KindEntra, Name: "adrient-test-entra", ClientID: "cid", ClientSecret: "sec", TenantID: "00000000-0000-0000-0000-000000000001", AutoCreate: true},
		{Kind: KindGoogle, Name: "adrient-test-google", ClientID: "cid", ClientSecret: "sec", AutoCreate: true},
		{Kind: KindOIDC, Name: "adrient-test-oidc", ClientID: "cid", ClientSecret: "sec", Issuer: c.URL, AutoCreate: true},
		{Kind: KindSAML, Name: "adrient-test-saml", MetadataXML: `<?xml version="1.0"?><EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.test/e"><IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol"><SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://idp.test/sso"/></IDPSSODescriptor></EntityDescriptor>`},
	} {
		id, err := c.CreateIdP(ctx, in)
		if err != nil {
			t.Errorf("%s: %v", in.Kind, err)
			continue
		}
		list, err := c.ListIdPs(ctx)
		found := false
		for _, p := range list {
			found = found || p.ID == id
		}
		if err != nil || !found {
			t.Errorf("%s: not listed (%v)", in.Kind, err)
		}
		if err := c.DeleteIdP(ctx, id); err != nil {
			t.Errorf("%s: delete: %v", in.Kind, err)
		}
	}
}
