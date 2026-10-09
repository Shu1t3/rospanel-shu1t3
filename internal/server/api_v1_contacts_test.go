package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAPIUserContactsAcrossPagedAndFilteredLists(t *testing.T) {
	t.Parallel()
	h, mgr, st := nodeAPITestServer(t)
	base, key := apiFixture(t, h, st)
	u, err := mgr.CreateUser(t.Context(), "contact", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetUserExternalID(t.Context(), u.ID, "contact-42"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetUserMailing(t.Context(), u.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetUserLang(t.Context(), u.ID, "en"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"?limit=1", "?search=contact", "?external_id=contact-42"} {
		rec := apiGet(t, h, base+"/v1/users"+query, key)
		var body struct {
			Data []userView `json:"data"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body.String())
		}
		if len(body.Data) != 1 {
			t.Fatalf("%s: users = %d", query, len(body.Data))
		}
		v := body.Data[0]
		if v.ID != u.ID || v.ExternalID != "contact-42" || v.Mailing == nil || *v.Mailing || v.Lang == nil || *v.Lang != "en" {
			t.Fatalf("%s: missing or incorrect contacts: %s", query, rec.Body.String())
		}
	}
}
