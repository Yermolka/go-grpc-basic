package main

import (
	"log"
	"net/http"
	"strings"

	"go-grpc-basic/proto"

	"github.com/gorilla/sessions"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var store = sessions.NewCookieStore([]byte("super-secret-key"))

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

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	session.Values["authenticated"] = false
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session-name")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	username := session.Values["username"].(string)
	w.Write([]byte("Welcome " + username + "! This is your dashboard."))
}

func loginPage(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`
		<html>
			<body>
				<form action="/login" method="post">
					Username: <input type="text" name="username"><br>
					Password: <input type="password" name="password"><br>
					<input type="submit" value="Login">
				</form>
			</body>
		</html>
	`))
}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewAuthServiceClient(conn)

	// Public routes
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			loginPage(w, r)
		} else {
			loginHandler(client).ServeHTTP(w, r)
		}
	})

	// Protected routes
	http.HandleFunc("/dashboard", authMiddleware(dashboardHandler))
	http.HandleFunc("/logout", logoutHandler)

	// Existing API endpoint
	http.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || strings.ToLower(tokenParts[0]) != "bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp, err := client.ValidateToken(r.Context(), &proto.ValidateTokenRequest{Token: tokenParts[1]})
		if err != nil {
			log.Printf("gRPC error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !resp.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authenticated!"))
	})

	log.Println("HTTP gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
