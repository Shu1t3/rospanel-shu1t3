package core

import (
	"context"
	"testing"
	"time"
)

func TestStandbyAndFencedMastersDoNotRenewFromBalance(t *testing.T) {
	for _, mode := range []string{"standby", "fenced"} {
		t.Run(mode, func(t *testing.T) {
			m, st, plan := walletFixture(t)
			u := walletUser(t, st, mode)
			if err := m.ApplyPlanToUser(context.Background(), u.ID, plan.ID, false); err != nil {
				t.Fatal(err)
			}
			credit(t, m, u.ID, 30000)
			soon := time.Now().Unix() + 1800
			if err := st.SetUserLimits(u.ID, 0, soon, 0); err != nil {
				t.Fatal(err)
			}
			if mode == "standby" {
				m.SetBillingStandby(true)
			} else {
				m.SetFenced(true)
			}
			if err := m.EnforceBilling(time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			after, err := st.GetUser(u.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.ExpireAt != soon || balanceOf(t, st, u.ID) != 30000 {
				t.Fatal("inactive master renewed or charged the user")
			}
			m.SetBillingStandby(false)
			m.SetFenced(false)
			if err := m.EnforceBilling(time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
			if balanceOf(t, st, u.ID) != 10100 {
				t.Fatal("active master did not resume renewal")
			}
		})
	}
}
