package store

import "testing"

func TestWebRegistrationRollbackAndRetry(t *testing.T) {
	t.Parallel()
	for _, moderated := range []bool{false, true} {
		t.Run(map[bool]string{false: "open", true: "moderated"}[moderated], func(t *testing.T) {
			st := newStore(t)
			ref, err := st.CreateUser("ref", "ref-uuid", "pw", "ref-token", 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			var reqID int64
			if moderated {
				req, err := st.CreateWebRegistrationRequest("client", "Client", "website", ref.ID, 1)
				if err != nil {
					t.Fatal(err)
				}
				reqID = req.ID
			}
			badPlan := UserPlanWrite{GroupIDs: []int64{999999}}
			in := RegistrationUser{Name: "Client", UUID: "client-uuid", Password: "pw", SubToken: "client-token", Plan: &badPlan}
			register := func() error {
				if moderated {
					_, _, err := st.ApproveWebRegistration(reqID, in)
					return err
				}
				_, err := st.CreateWebRegistration(in, "client", "website", ref.ID)
				return err
			}
			if err := register(); err == nil {
				t.Fatal("invalid plan succeeded")
			}
			if id, err := st.UserIDByExternalID("client"); err != nil || id != 0 {
				t.Fatalf("orphan identity: %d, %v", id, err)
			}
			if users, err := st.ListUsers(); err != nil || len(users) != 1 {
				t.Fatalf("orphan account: %v, %v", users, err)
			}
			if moderated {
				if req, err := st.GetRegistrationRequest(reqID); err != nil || req == nil {
					t.Fatalf("request lost: %v", err)
				}
			}
			in.Plan = nil
			if err := register(); err != nil {
				t.Fatalf("retry: %v", err)
			}
			id, _ := st.UserIDByExternalID("client")
			u, err := st.GetUser(id)
			w, walletErr := st.GetWalletLite(id)
			if err != nil || walletErr != nil || w.ReferrerID != ref.ID || st.UserSource(id) != "website" {
				t.Fatalf("registration data missing: %+v, %v", u, err)
			}
			if moderated {
				if req, _ := st.GetRegistrationRequestByExternal("client"); req != nil {
					t.Fatal("request survived approval")
				}
				if _, created, err := st.ApproveWebRegistration(reqID, in); err != nil || created {
					t.Fatalf("second approval: %v, %v", created, err)
				}
			} else if err := register(); err == nil {
				t.Fatal("duplicate identity succeeded")
			}
		})
	}
}

func TestApproveWebRegistrationKeepsExistingAccount(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	in := RegistrationUser{Name: "Client", UUID: "uuid", Password: "pw", SubToken: "token"}
	u, err := st.CreateWebRegistration(in, "client", "first", 0)
	if err != nil {
		t.Fatal(err)
	}
	req, err := st.CreateWebRegistrationRequest("client", "Other", "second", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, created, err := st.ApproveWebRegistration(req.ID, RegistrationUser{})
	if err != nil || created || got == nil || got.ID != u.ID {
		t.Fatalf("approval: %+v, %v, %v", got, created, err)
	}
	if st.UserSource(u.ID) != "first" {
		t.Fatal("existing account overwritten")
	}
	if pending, _ := st.GetRegistrationRequestByExternal("client"); pending != nil {
		t.Fatal("existing account request left pending")
	}
}
