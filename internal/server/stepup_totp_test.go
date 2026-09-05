package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/auth"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
)

// The two actions gated by a fresh second factor destroy something the panel cannot
// bring back: a factory reset wipes the database, and deleting a server cuts off
// everyone on it with no undo. These drive the real routes through the real mux,
// because the property that matters is "the handler refuses", not "the helper works".

// adminWithTOTP creates an owner with a second factor bound, and returns their
// session cookie plus the shared secret.
func adminWithTOTP(t *testing.T, st *store.Store, name string) (*http.Cookie, string) {
	t.Helper()
	c := signIn(t, st, name, model.RoleOwner, false)
	id, _, _, err := st.GetAdminAuth(name)
	if err != nil {
		t.Fatalf("lookup %s: %v", name, err)
	}
	return c, enrolTOTP(t, st, id)
}

// setupDone flips the first-run flag, because verifyStepUp waives re-authentication
// entirely while the wizard is still running.
func setupDone(t *testing.T, st *store.Store) {
	t.Helper()
	if err := st.SetSetupDone(true); err != nil {
		t.Fatalf("setup done: %v", err)
	}
}

// stepUp builds the credential body an irreversible action carries. A body, not
// headers: header values are ISO-8859-1 only, so a browser cannot send a non-ASCII
// password at all — which is the bug this shape exists to avoid.
func stepUp(password, code string) string {
	b, err := json.Marshal(map[string]string{"current_password": password, "code": code})
	if err != nil {
		panic(err)
	}
	return string(b)
}

func send(t *testing.T, rt *Router, method, path, body string, c *http.Cookie, hdr map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if c != nil {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	rt.panelMux().ServeHTTP(w, req)
	var env struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w.Code, env.Code
}

// Deleting a server cuts off users, so an admin who has bound an authenticator must
// hand over a fresh code before the panel acts on it.
func TestDeleteNodeRequiresFreshTOTP(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	cookie, secret := adminWithTOTP(t, st, "owner")

	// 1. Missing password outright.
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", `{}`, cookie, nil); errCode != "err.wrongPassword" {
		t.Fatalf("empty body: %d %s — want err.wrongPassword", code, errCode)
	}
	// 2. Right password, no code given.
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("a-password", ""), cookie, nil); errCode != "err.totpRequired" {
		t.Fatalf("password alone: %d %s — want err.totpRequired", code, errCode)
	}
	// 3. Wrong code.
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("a-password", "000000"), cookie, nil); errCode != "err.totpInvalid" {
		t.Fatalf("wrong code: %d %s — want err.totpInvalid", code, errCode)
	}
	// 4. Code already spent by the login. We sign in with it first to make it "spent",
	// exactly as happens when an admin logs in and opens the nodes tab.
	goodCode := codeNow(t, secret)
	if code, errCode := tryLogin(t, rt, `{"username":"owner","password":"a-password","code":"`+goodCode+`"}`); code != http.StatusOK {
		t.Fatalf("login with good code: %d %s", code, errCode)
	}
	// Re-attempting deletion with the very code that just signed the admin in.
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("a-password", goodCode), cookie, nil); errCode != "err.totpUsed" {
		t.Fatalf("reused login code: %d %s — want err.totpUsed", code, errCode)
	}
}

// When the admin has NOT turned on 2FA, the password alone must be enough: 2FA is an
// optional layer, not a prerequisite for deleting a node.
func TestDeleteNodePasswordOnlyWhenTOTPDisabled(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	cookie := signIn(t, st, "owner", model.RoleOwner, false)

	// Wrong password still refused.
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("wrong", ""), cookie, nil); errCode != "err.wrongPassword" {
		t.Fatalf("wrong password: %d %s", code, errCode)
	}
	// Right password passes the gate — the manager's "no such node" follows, which
	// proves the step-up accepted the request.
	code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("a-password", ""), cookie, nil)
	if errCode == "err.wrongPassword" || errCode == "err.totpRequired" {
		t.Fatalf("password-only step-up was refused: %d %s", code, errCode)
	}
}

// The first-run wizard waives the password step-up for ordinary settings, and that
// waiver must NOT extend to the two irreversible actions: the wizard clears the forced
// password change several steps before it marks setup done, so an abandoned wizard
// would otherwise leave a working panel where a session cookie alone deletes servers.
func TestIrreversibleActionsAreNotWaivedDuringSetup(t *testing.T) {
	rt, st := rolesTestRouter(t)
	// Deliberately NOT marking setup done.
	cookie := signIn(t, st, "owner", model.RoleOwner, false)

	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("wrong", ""), cookie, nil); errCode != "err.wrongPassword" {
		t.Errorf("node delete during setup: %d %s — want the password to be required", code, errCode)
	}
	body := `{"current_password":"wrong"}`
	if code, errCode := send(t, rt, http.MethodPost, "/api/reset", body, cookie, nil); errCode != "err.wrongPassword" {
		t.Errorf("factory reset during setup: %d %s — want the password to be required", code, errCode)
	}
}

// The factory reset asks for the same pair. It had the password already; the code is
// what is new, and the reset must not run without it.
func TestFactoryResetRequiresFreshTOTP(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	cookie, secret := adminWithTOTP(t, st, "owner")

	body := `{"current_password":"a-password"}`
	if code, errCode := send(t, rt, http.MethodPost, "/api/reset", body, cookie, nil); errCode != "err.totpRequired" {
		t.Fatalf("password alone: %d %s — want err.totpRequired", code, errCode)
	}
	bad := `{"current_password":"a-password","code":"000000"}`
	if code, errCode := send(t, rt, http.MethodPost, "/api/reset", bad, cookie, nil); errCode != "err.totpInvalid" {
		t.Fatalf("wrong code: %d %s — want err.totpInvalid", code, errCode)
	}

	// The reset itself is destructive and schedules a process restart, so this stops at
	// proving the gate opens: the step is claimed only after both credentials pass, so a
	// spent step is proof the handler was reached.
	good := codeNow(t, secret)
	id, _, _, err := st.GetAdminAuth("owner")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	step, ok := auth.VerifyTOTP(secret, good, time.Now(), 0)
	if !ok {
		t.Fatalf("the code we just generated does not verify")
	}
	claimed, err := st.MarkAdminTOTPStep(id, step)
	if err != nil || !claimed {
		t.Fatalf("pre-claim: claimed=%v err=%v", claimed, err)
	}
	// Now the same code is spent, which is exactly what a replay looks like.
	replay := `{"current_password":"a-password","code":"` + good + `"}`
	if code, errCode := send(t, rt, http.MethodPost, "/api/reset", replay, cookie, nil); errCode != "err.totpUsed" {
		t.Errorf("spent code answered %d %s, want err.totpUsed", code, errCode)
	}
}

// Guessing a six-digit code has to be throttled at the endpoint that acts on it. The
// attacker this counts already holds a session and the password, so a lockout anywhere
// else costs them nothing — and putting the count on the LOGIN counter cost the
// legitimate admin their login while leaving the guessing at full speed.
func TestStepUpTOTPThrottlesItselfAndNotTheLogin(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	cookie, secret := adminWithTOTP(t, st, "owner")
	bad := stepUp("a-password", "000000")

	var last string
	for i := 0; i < 12; i++ {
		_, last = send(t, rt, http.MethodDelete, "/api/nodes/1", bad, cookie, nil)
	}
	if last != "err.tooManyAttempts" {
		t.Errorf("twelve wrong codes ended with %q — the endpoint being guessed at is not throttled", last)
	}

	// The login form is a different counter: fumbling the delete dialog must not lock
	// the admin out of the panel itself.
	if code, errCode := tryLogin(t, rt, `{"username":"owner","password":"a-password","code":"`+codeNow(t, secret)+`"}`); code != http.StatusOK {
		t.Errorf("login after the step-up lockout: %d %s — want it unaffected", code, errCode)
	}
}

// A correct code clears the count, so a typo on the way to the right code costs
// nothing once it lands.
func TestStepUpTOTPSuccessClearsTheCount(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	cookie, secret := adminWithTOTP(t, st, "owner")

	bad := stepUp("a-password", "000000")
	for i := 0; i < 8; i++ {
		send(t, rt, http.MethodDelete, "/api/nodes/1", bad, cookie, nil)
	}
	good := stepUp("a-password", codeNow(t, secret))
	if code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", good, cookie, nil); errCode == "err.tooManyAttempts" {
		t.Fatalf("a correct code was refused by the lockout: %d %s", code, errCode)
	}
	// The counter is clear again: another wrong code must not be an instant lockout.
	if _, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", bad, cookie, nil); errCode != "err.totpInvalid" {
		t.Errorf("after a success the count was not cleared: %s", errCode)
	}
}

// A password is any string an admin chose; nothing restricts it to ASCII. It has to
// survive the trip, which is why the credentials ride in a JSON body — an HTTP header
// is ISO-8859-1, so a browser refuses a Cyrillic password outright and turns an
// accented one into bytes that can never match the stored hash.
func TestStepUpAcceptsANonASCIIPassword(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	const pw = "пароль-Ω-123"
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	id, err := st.CreateAdmin("owner", hash, model.RoleOwner, false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := st.CreateSession(id, time.Hour)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: token}

	// The right password gets past the gate; the manager's own answer follows.
	code, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp(pw, ""), cookie, nil)
	if errCode == "err.wrongPassword" {
		t.Fatalf("a correct non-ASCII password was rejected: %d %s", code, errCode)
	}
	// And a wrong one is still wrong.
	if _, errCode := send(t, rt, http.MethodDelete, "/api/nodes/1", stepUp("пароль-Ω-124", ""), cookie, nil); errCode != "err.wrongPassword" {
		t.Errorf("a wrong non-ASCII password was accepted: %s", errCode)
	}
}

// Removing an admin re-checks the owner's password, and that password is whatever the
// owner chose — nothing restricts it to ASCII. It rides in the body for the same reason
// the irreversible actions do: a header is ISO-8859-1, so a browser cannot send a
// Cyrillic password at all, and an owner with one could never remove an account.
func TestDeleteAdminAcceptsANonASCIIPassword(t *testing.T) {
	rt, st := rolesTestRouter(t)
	setupDone(t, st)
	const pw = "Владелец-Ω-9"
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ownerID, err := st.CreateAdmin("owner", hash, model.RoleOwner, false)
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	token, err := st.CreateSession(ownerID, time.Hour)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: token}

	victimHash, _ := auth.HashPassword("whatever-1")
	victimID, err := st.CreateAdmin("support", victimHash, model.RoleOperator, false)
	if err != nil {
		t.Fatalf("create victim: %v", err)
	}
	path := "/api/admins/" + strconv.FormatInt(victimID, 10)

	// A wrong password is still refused.
	if _, errCode := send(t, rt, http.MethodDelete, path, `{"current_password":"Владелец-Ω-8"}`, cookie, nil); errCode != "err.wrongPassword" {
		t.Fatalf("a wrong non-ASCII password was accepted: %s", errCode)
	}
	// The right one goes through.
	body := `{"current_password":"` + pw + `"}`
	if code, errCode := send(t, rt, http.MethodDelete, path, body, cookie, nil); code != http.StatusOK {
		t.Fatalf("a correct non-ASCII password was rejected: %d %s", code, errCode)
	}
	if _, err := st.GetAdmin(victimID); err == nil {
		t.Error("the account was not removed")
	}
}
