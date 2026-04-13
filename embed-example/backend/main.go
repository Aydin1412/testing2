package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// ==============================
// REQUEST STRUCT (FROM FRONTEND)
// ==============================
type GuestTokenRequest struct {
	DashboardID string `json:"dashboardId"`
	Tenant      string `json:"tenant"` // 🔥 dynamic tenant
}

// ==============================
// STEP 1: LOGIN → ACCESS TOKEN
// ==============================
func getAccessToken(baseURL string, jar *cookiejar.Jar) (string, error) {
	payload := map[string]interface{}{
		"username": "admin", // ⚠️ change in production
		"password": "admin",
		"provider": "db",
		"refresh":  true,
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/security/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	token, ok := result["access_token"].(string)
	if !ok {
		return "", errors.New("failed to get access_token")
	}

	return token, nil
}

// ==============================
// STEP 2: GET CSRF TOKEN
// ==============================
func getCSRFToken(baseURL, accessToken string, jar *cookiejar.Jar) (string, error) {
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/security/csrf_token/", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	csrf, ok := result["result"].(string)
	if !ok {
		return "", errors.New("failed to get csrf token")
	}

	return csrf, nil
}

// ==============================
// MAIN HANDLER
// ==============================
func guestTokenHandler(w http.ResponseWriter, r *http.Request) {

	// ==========================
	// CORS
	// ==========================
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// ==========================
	// PARSE BODY
	// ==========================
	var reqBody GuestTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if reqBody.DashboardID == "" {
		http.Error(w, "dashboardId required", http.StatusBadRequest)
		return
	}

	if reqBody.Tenant == "" {
		http.Error(w, "tenant required", http.StatusBadRequest)
		return
	}

	supersetBase := os.Getenv("SUPERSET_BASE_URL")
	if supersetBase == "" {
		http.Error(w, "SUPERSET_BASE_URL not set", http.StatusInternalServerError)
		return
	}

	// ==========================
	// COOKIE JAR
	// ==========================
	jar, _ := cookiejar.New(nil)

	// ==========================
	// LOGIN
	// ==========================
	accessToken, err := getAccessToken(supersetBase, jar)
	if err != nil {
		log.Println("login failed:", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}

	// ==========================
	// CSRF
	// ==========================
	csrfToken, err := getCSRFToken(supersetBase, accessToken, jar)
	if err != nil {
		log.Println("csrf failed:", err)
		http.Error(w, "csrf failed", http.StatusInternalServerError)
		return
	}

	// ==========================
	// 🔥 BUILD RLS CLAUSE
	// ==========================
	tenant := reqBody.Tenant
	rlsClause := "tenant = '" + tenant + "'"

	log.Println("Applying RLS:", rlsClause)

	// ==========================
	// STEP 3: REQUEST GUEST TOKEN
	// ==========================
	payload := map[string]interface{}{
		"resources": []map[string]interface{}{
			{"type": "dashboard", "id": reqBody.DashboardID},
		},

		// 🔥 DYNAMIC RLS HERE
		"rls": []map[string]interface{}{
			{
				"clause": rlsClause,
			},
		},

		"user": map[string]interface{}{
			"username": "user_" + tenant,
		},
	}

	payloadBytes, _ := json.Marshal(payload)

	supReq, _ := http.NewRequest(
		"POST",
		supersetBase+"/api/v1/security/guest_token/",
		bytes.NewBuffer(payloadBytes),
	)

	supReq.Header.Set("Authorization", "Bearer "+accessToken)
	supReq.Header.Set("X-CSRFToken", csrfToken)
	supReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(supReq)
	if err != nil {
		log.Println("guest_token failed:", err)
		http.Error(w, "superset error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		log.Println("superset error:", string(body))
		http.Error(w, string(body), http.StatusBadGateway)
		return
	}

	// ==========================
	// RETURN TOKEN
	// ==========================
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// ==============================
// MAIN
// ==============================
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	http.HandleFunc("/superset/guest-token", guestTokenHandler)

	log.Println("Server running on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}