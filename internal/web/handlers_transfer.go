package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
)

// Moving a login to another squadron. See auth/transfers.go for the rules. This file
// is the pages, the emails and the security-log lines.

// transferMessage turns the service's errors into words for the page.
func transferMessage(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrTransferGone):
		return http.StatusNotFound, "That move is no longer waiting: it was decided, cancelled or has run out of time."
	case errors.Is(err, auth.ErrTransferNotAllowed):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, auth.ErrNotMember):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, auth.ErrTransferOfficial), errors.Is(err, auth.ErrTransferOnlyOwner),
		errors.Is(err, auth.ErrTransferNoTarget), errors.Is(err, auth.ErrTransferSameSquadron), errors.Is(err, auth.ErrTransferOpen),
		errors.Is(err, auth.ErrTransferInvite), errors.Is(err, auth.ErrTransferNoOwner), errors.Is(err, auth.ErrTransferRole), errors.Is(err, auth.ErrTransferTemp):
		return http.StatusBadRequest, err.Error()
	}
	log.Printf("account transfer: %v", err)
	return http.StatusInternalServerError, "Something went wrong. Try again."
}

func (s *server) absoluteURL(path string) string {
	return strings.TrimSuffix(s.Cfg.BaseURL, "/") + s.url(path)
}

// transferNotify emails everyone a step concerns and writes the security-log lines of
// both squadrons. event is requested, agreed, accepted, declined, cancelled or expired.
// Failures are logged: a move does not wait on, or fail because of, mail.
func (s *server) transferNotify(event string, t *auth.Transfer, actorID int64, actorEmail string) {
	detail := fmt.Sprintf("%s: %s, from squadron %s to squadron %s", event, t.Email, t.FromNumber, t.ToNumber)
	if event == "accepted" {
		detail += " as " + t.Role
	}
	for _, sq := range []int64{t.FromID, t.ToID} {
		if err := s.Auth.Audit(sq, actorID, actorEmail, auth.AuditTransfer, detail); err != nil {
			log.Printf("audit transfer for squadron %d: %v", sq, err)
		}
	}
	from, to := t.FromNumber+" "+t.FromName, t.ToNumber+" "+t.ToName
	send := func(addr, subject, body string) {
		if err := s.Mail.SendNotice(addr, subject, body); err != nil {
			log.Printf("transfer mail to %s: %v", addr, err)
		}
	}
	owners := func(squadronID int64, subject, body string) {
		list, err := s.Auth.TransferOwners(squadronID)
		if err != nil {
			log.Printf("transfer owners of %d: %v", squadronID, err)
			return
		}
		for _, addr := range list {
			if !strings.EqualFold(addr, t.Email) {
				send(addr, subject, body)
			}
		}
	}
	link := s.absoluteURL("/transfer/" + t.PersonalToken)
	squadronPage := s.absoluteURL("/squadron#transfers")
	days := int(auth.TransferLifetime / (24 * time.Hour))
	switch event {
	case "requested":
		if t.ByOwner {
			send(t.Email, "An owner asks to move your RAFACTools Planner login to another squadron",
				fmt.Sprintf("An owner of %s has asked to move your login to %s.\n\nNothing happens unless you agree. To agree or decline, open:\n%s\n\nIf you agree, your account is switched off until an owner of %s accepts you (and gives you a role there), and you then keep your password and name. You keep nothing of %s's programmes: they stay with that squadron. The request ends after %d days.\n\nIf you did not expect this, decline it or ignore this email.\n", from, to, link, to, from, days))
			return
		}
		send(t.Email, "You asked to move your RAFACTools Planner login to another squadron",
			fmt.Sprintf("You asked to move your login from %s to %s.\n\nYour account is switched off until an owner of %s accepts you. If they accept, you can sign in again (as a member of %s). If they decline, or %d days pass, your account is switched back on in %s.\n\nTo cancel the move, open:\n%s\n\nIf you did not ask for this, cancel it and change your password.\n", from, to, to, to, days, from, link))
		s.transferToOwners(owners, t, squadronPage, from, to, days)
		owners(t.FromID, "A member of your squadron asked to move to another squadron",
			fmt.Sprintf("%s asked to move their login from %s to %s. Their account is switched off until an owner of %s decides. You can cancel the move under Squadron.\n", t.Name(), from, to, to))
	case "agreed":
		s.transferToOwners(owners, t, squadronPage, from, to, days)
		owners(t.FromID, "A member of your squadron agreed to move to another squadron",
			fmt.Sprintf("%s agreed to move their login from %s to %s. Their account is switched off until an owner of %s decides. You can cancel the move under Squadron.\n", t.Name(), from, to, to))
	case "accepted":
		send(t.Email, "Your RAFACTools Planner login has moved",
			fmt.Sprintf("An owner of %s accepted you as %s. Sign in again at %s with the same email address and password.\n\nYou no longer see anything of %s.\n", to, t.Role, s.absoluteURL("/login"), from))
		owners(t.FromID, "A member has moved to another squadron",
			fmt.Sprintf("%s has moved their login from %s to %s. They no longer see your squadron.\n", t.Name(), from, to))
		owners(t.ToID, "A member has joined your squadron",
			fmt.Sprintf("%s has joined %s as %s, moved from %s.\n", t.Name(), to, t.Role, from))
	case "declined", "cancelled", "expired":
		why := map[string]string{"declined": "was declined", "cancelled": "was cancelled", "expired": "ran out of time"}[event]
		send(t.Email, "Your RAFACTools Planner move "+why,
			fmt.Sprintf("The move of your login from %s to %s %s. Your account is switched on again in %s: sign in at %s as before.\n", from, to, why, from, s.absoluteURL("/login")))
		owners(t.FromID, "A move to another squadron "+why,
			fmt.Sprintf("The move of %s from %s to %s %s. Their account is switched on again.\n", t.Name(), from, to, why))
		if event != "declined" && t.WasPending {
			owners(t.ToID, "A request to join your squadron "+why,
				fmt.Sprintf("The request from %s to move from %s to %s %s.\n", t.Name(), from, to, why))
		}
	}
}

func (s *server) transferToOwners(owners func(int64, string, string), t *auth.Transfer, page, from, to string, days int) {
	owners(t.ToID, "Someone asks to join your squadron on RAFACTools Planner",
		fmt.Sprintf("%s (%s) asks to move their login from %s to %s.\n\nAn owner can accept (and choose their role) or decline under Squadron:\n%s\n\nNothing moves unless you accept. Only the login moves: none of %s's programmes or history come with them. The request ends after %d days.\n", t.Name(), t.Email, from, to, page, from, days))
}

// --- The person's own page -------------------------------------------------

func (s *server) accountTransferPage(w http.ResponseWriter, r *http.Request) {
	s.renderAccountTransfer(w, r, http.StatusOK, "")
}

func (s *server) renderAccountTransfer(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	u := userFrom(r.Context())
	open, err := s.Auth.OpenTransferFor(u.ID)
	if err != nil {
		log.Printf("open transfer: %v", err)
		s.renderError(w, r, http.StatusInternalServerError, "Move to another squadron", "Something went wrong.")
		return
	}
	cannot := ""
	if why := s.Auth.MovableReason(u.ID); why != nil && open == nil {
		cannot = why.Error()
	}
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	s.render(w, r, "account_transfer", "Move to another squadron", map[string]any{"Open": open, "Cannot": cannot, "Error": errMsg})
}

// accountTransferStart is a person moving themself. Their account is switched off at
// once, so they are signed out and sent to the sign-in page.
func (s *server) accountTransferStart(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Move to another squadron", "Form expired, try again.")
		return
	}
	if !s.turnstileOK(r) {
		s.renderAccountTransfer(w, r, http.StatusBadRequest, turnstileFailMsg)
		return
	}
	u := userFrom(r.Context())
	if u.ViewingOther || u.IsMasterAdmin {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "This account cannot be moved.")
		return
	}
	if _, err := s.Auth.Authenticate(u.Email, r.FormValue("password")); err != nil {
		s.renderAccountTransfer(w, r, http.StatusBadRequest, "Your password is not right.")
		return
	}
	t, err := s.Auth.StartTransfer(u.ID, u.ID, r.FormValue("number"))
	if err != nil {
		status, msg := transferMessage(err)
		s.renderAccountTransfer(w, r, status, msg)
		return
	}
	s.transferNotify("requested", t, u.ID, u.Email)
	s.setSessionCookie(w, "", time.Time{})
	http.Redirect(w, r, s.url("/login?transfer=1"), http.StatusSeeOther)
}

// accountTransferAnswer is a signed-in person agreeing to or declining a move an owner
// asked for. They come through the emailed link as well; this is the same thing for
// someone who is signed in.
func (s *server) accountTransferAnswer(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Move to another squadron", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	open, err := s.Auth.OpenTransferFor(u.ID)
	if err != nil || open == nil || open.Status != auth.TransferAwaitingPerson {
		s.renderError(w, r, http.StatusNotFound, "Move to another squadron", "There is no move waiting for you to agree to.")
		return
	}
	if r.FormValue("answer") == "decline" {
		t, err := s.Auth.CancelTransfer(open.ID, u.ID, "")
		if err != nil {
			status, msg := transferMessage(err)
			s.renderAccountTransfer(w, r, status, msg)
			return
		}
		s.transferNotify("declined", t, u.ID, u.Email)
		http.Redirect(w, r, s.url("/account/transfer"), http.StatusSeeOther)
		return
	}
	t, err := s.Auth.ConfirmTransferFor(u.ID)
	if err != nil {
		status, msg := transferMessage(err)
		s.renderAccountTransfer(w, r, status, msg)
		return
	}
	s.transferNotify("agreed", t, u.ID, u.Email)
	s.setSessionCookie(w, "", time.Time{}) // the account is switched off now
	http.Redirect(w, r, s.url("/login?transfer=1"), http.StatusSeeOther)
}

// --- The owner of the squadron the person leaves ----------------------------

func (s *server) memberTransfer(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	memberID, ok := parseID(r, "memberID")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid member.")
		return
	}
	if !s.turnstileOK(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", turnstileFailMsg)
		return
	}
	if memberID == u.ID {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "To move your own login use Your details, then Move to another squadron.")
		return
	}
	// Only someone in this squadron can be moved from here.
	found := false
	if members, err := s.Auth.ListMembers(u.SquadronID); err == nil {
		for _, m := range members {
			found = found || m.ID == memberID
		}
	}
	if !found {
		s.renderError(w, r, http.StatusNotFound, "Not found", auth.ErrNotMember.Error())
		return
	}
	t, err := s.Auth.StartTransfer(memberID, u.ID, r.FormValue("number"))
	if err != nil {
		status, msg := transferMessage(err)
		s.renderError(w, r, status, "Squadron settings", msg)
		return
	}
	s.transferNotify("requested", t, u.ID, u.Email)
	http.Redirect(w, r, s.url("/squadron#transfers"), http.StatusSeeOther)
}

// transferDecide is an owner accepting or declining a move into their squadron, or
// cancelling a move out of it.
func (s *server) transferDecide(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Squadron settings", "Form expired, try again.")
		return
	}
	u := userFrom(r.Context())
	id, ok := parseID(r, "id")
	if !ok {
		s.renderError(w, r, http.StatusNotFound, "Not found", "Invalid request.")
		return
	}
	if u.ViewingOther {
		s.renderError(w, r, http.StatusForbidden, "Forbidden", "You are looking at this squadron read-only.")
		return
	}
	var t *auth.Transfer
	var err error
	event := r.PathValue("action")
	switch event {
	case "accept":
		event = "accepted"
		t, err = s.Auth.AcceptTransfer(id, u.ID, r.FormValue("role"))
	case "decline":
		event = "declined"
		t, err = s.Auth.DeclineTransfer(id, u.ID, "")
	case "cancel":
		event = "cancelled"
		t, err = s.Auth.CancelTransfer(id, u.ID, "")
	default:
		s.renderError(w, r, http.StatusNotFound, "Not found", "Unknown action.")
		return
	}
	if err != nil {
		status, msg := transferMessage(err)
		s.renderError(w, r, status, "Squadron settings", msg)
		return
	}
	s.transferNotify(event, t, u.ID, u.Email)
	http.Redirect(w, r, s.url("/squadron#transfers"), http.StatusSeeOther)
}

// --- The person's emailed link (works while the account is switched off) ----

func (s *server) transferLinkPage(w http.ResponseWriter, r *http.Request) {
	t, err := s.Auth.TransferByToken(r.PathValue("token"))
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Move to another squadron", "That link is not valid.")
		return
	}
	s.render(w, r, "transfer_link", "Move to another squadron", map[string]any{
		"T": t, "Token": r.PathValue("token"), "Waiting": t.Status == auth.TransferAwaitingPerson || t.Status == auth.TransferPending,
		"AwaitingPerson": t.Status == auth.TransferAwaitingPerson, "Pending": t.Status == auth.TransferPending,
	})
}

func (s *server) transferLinkAct(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Move to another squadron", "Form expired, try again.")
		return
	}
	token := r.PathValue("token")
	t, err := s.Auth.TransferByToken(token)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "Move to another squadron", "That link is not valid.")
		return
	}
	event := ""
	switch r.PathValue("action") {
	case "agree":
		event = "agreed"
		t, err = s.Auth.ConfirmTransfer(token)
	case "decline":
		event = "declined"
		t, err = s.Auth.DeclineTransfer(t.ID, 0, token)
	case "cancel":
		event = "cancelled"
		t, err = s.Auth.CancelTransfer(t.ID, 0, token)
	default:
		s.renderError(w, r, http.StatusNotFound, "Not found", "Unknown action.")
		return
	}
	if err != nil {
		status, msg := transferMessage(err)
		s.renderError(w, r, status, "Move to another squadron", msg)
		return
	}
	t.PersonalToken = token
	s.transferNotify(event, t, t.UserID, t.Email)
	http.Redirect(w, r, s.url("/transfer/"+token), http.StatusSeeOther)
}

// expireTransfers ends moves that have run out of time and tells the people involved.
func (s *server) ExpireTransfers() {
	list, err := s.Auth.ExpireTransfers(time.Now())
	if err != nil {
		log.Printf("expire transfers: %v", err)
		return
	}
	for i := range list {
		s.transferNotify("expired", &list[i], 0, "system")
	}
}

// transferView is what the Squadron page shows an owner.
type transferView struct {
	auth.Transfer
	Expires string
}

func (s *server) transferViews(list []auth.Transfer, err error) []transferView {
	if err != nil {
		log.Printf("list transfers: %v", err)
		return nil
	}
	out := make([]transferView, 0, len(list))
	for _, t := range list {
		out = append(out, transferView{t, t.ExpiresAt.In(time.Local).Format("2 Jan 2006")})
	}
	return out
}

// sweepTransfers ends moves that have run out of time (switching the accounts back on)
// when the site starts and then every hour until ctx ends.
func (s *server) sweepTransfers(ctx context.Context) {
	s.ExpireTransfers()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.ExpireTransfers()
		}
	}
}
