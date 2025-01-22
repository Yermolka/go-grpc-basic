package main

import "net/http"

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	session.Values["authenticated"] = false
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	renderTemplate(w, "dashboard", PageData{
		Title: "Dashboard",
		Data: struct{ Username string }{
			Username: session.Values["username"].(string),
		},
	})
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", PageData{
		Title: "Login Page",
	})
}
