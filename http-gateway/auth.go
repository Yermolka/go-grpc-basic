package main

import (
	"go-grpc-basic/proto"
	"net/http"
)

func loginHandler(client proto.AuthServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")

		resp, err := client.AuthenticateUser(r.Context(), &proto.AuthenticateUserRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			http.Error(w, "Authentication failed", http.StatusInternalServerError)
			return
		}

		if !resp.Success {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		session, _ := store.Get(r, "session-name")
		session.Values["authenticated"] = true
		session.Values["username"] = username
		session.Save(r, w)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}
