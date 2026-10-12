package httpapi

import "net/http"

func (a *API) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Auth.RequestPasswordReset(r.Context(), in.Email, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	write(w, 202, map[string]string{"message": "If a verified account exists for that email, a password reset link will be sent. Check your inbox and spam folder."})
}
func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Auth.ResetPassword(r.Context(), in.Token, in.Password, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current  string `json:"current_password"`
		Password string `json:"new_password"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.Auth.ChangePassword(r.Context(), currentUser(r).ID, sessionCredential(a, r), in.Current, in.Password, requestID(r)); err != nil {
		a.fail(w, r, err)
		return
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
