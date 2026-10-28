package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"rafactools-planner/internal/auth"
)

var secretInPage = regexp.MustCompile(`<code data-private>([A-Z2-7 ]{20,})</code>`)

// currentCode computes the authenticator code for a base32 secret, the way a
// phone app would.
func currentCode(t *testing.T, secret string, offset int64) string {
	t.Helper()
	code, err := auth.TOTPCodeForTest(strings.ReplaceAll(secret, " ", ""), time.Now(), offset)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestMasterAdminTwoStepSignIn(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	app := ts.a

	page := body(master.get("/account"))
	if !strings.Contains(page, "Set up two-step sign-in") {
		t.Fatalf("master's page does not offer two-step:\n%s", page)
	}

	// Ordinary users are neither offered it nor allowed to post for it.
	if strings.Contains(body(ts.ownerA.get("/account")), "Two-step sign-in") {
		t.Error("an ordinary user is offered two-step sign-in")
	}
	if got := ts.ownerA.post("/account/2fa/start", url.Values{}).StatusCode; got != http.StatusForbidden {
		t.Errorf("ordinary user start = %d, want 403", got)
	}

	// Enrol: start, read the key, confirm with a code.
	wantRedirect(t, master.post("/account/2fa/start", url.Values{}), "/account")
	m := secretInPage.FindStringSubmatch(body(master.get("/account")))
	if m == nil {
		t.Fatal("secret not shown after starting")
	}
	secret := m[1]
	if got := master.post("/account/2fa/confirm", url.Values{"code": {"000000"}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("wrong code at confirm = %d, want 400", got)
	}
	wantRedirect(t, master.post("/account/2fa/confirm", url.Values{"code": {currentCode(t, secret, 0)}}), "/account?twostep=on")

	// A fresh browser now needs the code as well as the password.
	signIn := func(code string) int {
		c := app.anon()
		tok := scrapeCSRF(t, c.get("/login"))
		f := url.Values{"csrf": {tok}, "email": {"master@example.org"}, "password": {testPassword}}
		if code != "" {
			f.Set("code", code)
		}
		return c.postRaw("/login", f).StatusCode
	}
	if got := signIn(""); got != http.StatusUnauthorized {
		t.Errorf("password alone = %d, want 401", got)
	}
	if got := signIn("000000"); got != http.StatusUnauthorized {
		t.Errorf("wrong code = %d, want 401", got)
	}
	good := currentCode(t, secret, 1)
	if got := signIn(good); got != http.StatusSeeOther {
		t.Errorf("right code = %d, want 303", got)
	}
	if got := signIn(good); got != http.StatusUnauthorized {
		t.Errorf("the same code again = %d, want 401 (single use)", got)
	}

	// Other people sign in as before, with no code.
	if got := app.login(ts.ownerAEmail).get("/").StatusCode; got != http.StatusOK {
		t.Error("an ordinary user's login changed")
	}

	// Turning it off needs the password and a fresh code. Real time moves on by
	// 30 seconds between codes; here the used-step marker is wound back instead.
	app.db.Exec(`UPDATE users SET totp_last_step = 0 WHERE email = 'master@example.org'`)
	if got := master.post("/account/2fa/disable", url.Values{"password": {"wrong password!!"}, "code": {currentCode(t, secret, 1)}}).StatusCode; got != http.StatusBadRequest {
		t.Errorf("disable with wrong password = %d, want 400", got)
	}
	wantRedirect(t, master.post("/account/2fa/disable", url.Values{"password": {testPassword}, "code": {currentCode(t, secret, 1)}}), "/account?twostep=off")
	if got := signIn(""); got != http.StatusSeeOther {
		t.Errorf("after turning it off, password alone = %d, want 303", got)
	}
}

func TestWrongCodesLockTwoStepSignInTemporarily(t *testing.T) {
	ts, master, _, _ := masterApp(t)
	app := ts.a
	wantRedirect(t, master.post("/account/2fa/start", url.Values{}), "/account")
	secret := secretInPage.FindStringSubmatch(body(master.get("/account")))[1]
	wantRedirect(t, master.post("/account/2fa/confirm", url.Values{"code": {currentCode(t, secret, 0)}}), "/account?twostep=on")

	attempt := func(code string) (int, string) {
		c := app.anon()
		f := url.Values{"csrf": {scrapeCSRF(t, c.get("/login"))}, "email": {"master@example.org"}, "password": {testPassword}, "code": {code}}
		// each attempt comes from a different address so the per-address rate
		// limit is not what stops it
		req, _ := http.NewRequest("POST", c.base+"/login", strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "203.0.113."+strconv.Itoa(len(code)+int(time.Now().UnixNano()%200)))
		resp := c.do(req)
		return resp.StatusCode, body(resp)
	}
	for i := 0; i < 5; i++ {
		if got, _ := attempt("000000"); got != http.StatusUnauthorized {
			t.Fatalf("wrong code %d = %d, want 401", i, got)
		}
	}
	got, msg := attempt(currentCode(t, secret, 1))
	if got != http.StatusUnauthorized || !strings.Contains(msg, "too many wrong codes") {
		t.Errorf("a right code while locked = %d %q, want 401 with the lock message", got, msg)
	}
}
