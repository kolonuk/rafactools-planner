package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"rafactools-planner/internal/auth"
)

func (s *server) claimForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "claim", "Claim your squadron", map[string]any{"ClaimDomain": s.Auth.ClaimDomain()})
}

func (s *server) claimSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Claim your squadron", "Form expired, try again.")
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")
	squadronName := r.FormValue("squadron_name")

	_, token, err := s.Auth.ClaimSquadron(email, password, squadronName)
	if err != nil {
		s.render(w, r, "claim", "Claim your squadron", map[string]any{
			"ClaimDomain": s.Auth.ClaimDomain(),
			"Error":       err.Error(),
		})
		return
	}

	// token is "" when SKIP_EMAIL_VERIFICATION is set (dev only — see
	// config.Config.SkipEmailVerification): the account is already verified,
	// there's no token to email, and no email gets sent at all.
	if token == "" {
		http.Redirect(w, r, s.url("/login?claimed=1"), http.StatusSeeOther)
		return
	}

	if err := s.Mail.SendVerification(email, s.Cfg.BaseURL, token); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Claim your squadron", "Account created, but the confirmation email failed to send. Contact an admin.")
		return
	}
	http.Redirect(w, r, s.url("/check-email?email="+url.QueryEscape(email)), http.StatusSeeOther)
}

func (s *server) checkEmail(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "check_email", "Check your email", map[string]any{"Email": r.URL.Query().Get("email")})
}

func (s *server) verify(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := s.Auth.VerifyEmail(token); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Verify email", "That link is invalid or has expired.")
		return
	}
	http.Redirect(w, r, s.url("/login?verified=1"), http.StatusSeeOther)
}

func (s *server) loginForm(w http.ResponseWriter, r *http.Request) {
	extra := map[string]any{}
	switch {
	case r.URL.Query().Get("verified") == "1":
		extra["Flash"] = "Email confirmed — you can log in now."
	case r.URL.Query().Get("reset") == "1":
		extra["Flash"] = "Password changed. Log in with your new password."
	case r.URL.Query().Get("claimed") == "1":
		extra["Flash"] = "Squadron claimed — you can log in now (dev mode: no email verification)."
	}
	s.render(w, r, "login", "Log in", extra)
}

func (s *server) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Log in", "Form expired, try again.")
		return
	}
	user, err := s.Auth.Authenticate(r.FormValue("email"), r.FormValue("password"))
	if err != nil {
		msg := "Invalid email or password."
		if errors.Is(err, auth.ErrEmailNotVerified) {
			msg = "Confirm your email first — check your inbox for the verification link."
		}
		s.renderError(w, r, http.StatusUnauthorized, "Log in", msg)
		return
	}

	token, expiresAt, err := s.Auth.CreateSession(user.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Log in", "Something went wrong.")
		return
	}
	s.setSessionCookie(w, token, expiresAt)
	http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
		return
	}
	_ = s.Auth.DeleteSession(s.sessionToken(r))
	s.setSessionCookie(w, "", time.Time{})
	http.Redirect(w, r, s.url("/login"), http.StatusSeeOther)
}

func (s *server) inviteForm(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	inv, err := s.Auth.GetInvite(token)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Invite", "That invite link is invalid or has expired.")
		return
	}
	sq, err := s.Store.GetSquadron(inv.SquadronID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Invite", "Something went wrong.")
		return
	}
	s.render(w, r, "invite_accept", "Join "+sq.Name, map[string]any{
		"Token": inv.Token, "Email": inv.Email, "Role": inv.Role, "SquadronName": sq.Name,
	})
}

func (s *server) inviteSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Invite", "Form expired, try again.")
		return
	}
	token := r.PathValue("token")
	userID, _, err := s.Auth.AcceptInvite(token, r.FormValue("password"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Invite", err.Error())
		return
	}
	sessToken, expiresAt, err := s.Auth.CreateSession(userID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Invite", "Account created — please log in.")
		return
	}
	s.setSessionCookie(w, sessToken, expiresAt)
	http.Redirect(w, r, s.url("/"), http.StatusSeeOther)
}

func (s *server) forgotForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "forgot", "Forgot your password", nil)
}

// forgotSubmit answers the same way whether or not the address has an
// account, and sends the email in the background so the response time does
// not give the answer away either.
func (s *server) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Forgot your password", "Form expired, try again.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	token, send, err := s.Auth.RequestPasswordReset(email)
	if err != nil {
		log.Printf("password reset request: %v", err)
	} else if send {
		go func() {
			if err := s.Mail.SendPasswordReset(email, s.Cfg.BaseURL, token); err != nil {
				log.Printf("password reset email: %v", err)
			}
		}()
	}
	s.render(w, r, "forgot", "Check your email", map[string]any{"Sent": true})
}

func (s *server) resetForm(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := s.Auth.CheckPasswordReset(token); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Reset password", "That password reset link is invalid or has expired. You can ask for a new one.")
		return
	}
	s.render(w, r, "reset", "Choose a new password", map[string]any{"Token": token})
}

func (s *server) resetSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.checkCSRF(r) {
		s.renderError(w, r, http.StatusBadRequest, "Reset password", "Form expired, try again.")
		return
	}
	token := r.PathValue("token")
	err := s.Auth.ResetPassword(token, r.FormValue("password"))
	switch {
	case errors.Is(err, auth.ErrResetInvalid):
		s.renderError(w, r, http.StatusBadRequest, "Reset password", "That password reset link is invalid or has expired. You can ask for a new one.")
		return
	case err != nil:
		// A password the rules refuse leaves the link usable, so show the
		// form again with the reason.
		if r2 := s.Auth.CheckPasswordReset(token); r2 != nil {
			s.renderError(w, r, http.StatusBadRequest, "Reset password", "Something went wrong. Ask for a new link.")
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, r, "reset", "Choose a new password", map[string]any{"Token": token, "Error": err.Error()})
		return
	}
	http.Redirect(w, r, s.url("/login?reset=1"), http.StatusSeeOther)
}
