package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// ==============================
// REQUEST STRUCT (FROM FRONTEND)
// ==============================
type GuestTokenRequest struct {
	DashboardID string `json:"dashboardId"`
}

// ==============================
// STEP 1: LOGIN → ACCESS TOKEN
// ==============================
func getAccessToken(baseURL string, jar *cookiejar.Jar) (string, error) {
	// Dynamically pull from .env
	username := os.Getenv("SUPERSET_USERNAME")
	password := os.Getenv("SUPERSET_PASSWORD")

	if username == "" || password == "" {
		return "", errors.New("SUPERSET_USERNAME and SUPERSET_PASSWORD must be set in .env")
	}

	payload := map[string]interface{}{
		"username": username,
		"password": password,
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
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "*" // Fallback to allow all if not specified
	}

	// ==========================
	// CORS
	// ==========================
	w.Header().Set("Access-Control-Allow-Origin", frontendURL)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// ==========================
	// 1. EXTRACT JWT FROM HEADER
	// ==========================
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
		return
	}
	fossilToken := strings.TrimPrefix(authHeader, "Bearer ")

	// ==========================
	// 2. PARSE JWT PAYLOAD
	// ==========================
	tokenParts := strings.Split(fossilToken, ".")
	if len(tokenParts) != 3 {
		http.Error(w, "Invalid JWT format", http.StatusUnauthorized)
		return
	}

	// Decode Base64 payload
	payloadBytesDecoded, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if err != nil {
		log.Println("Error decoding JWT:", err)
		http.Error(w, "Invalid JWT payload", http.StatusUnauthorized)
		return
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytesDecoded, &claims); err != nil {
		http.Error(w, "Failed to parse JWT claims", http.StatusUnauthorized)
		return
	}

	// Extract Tenant and Subtenant
	fossilTenant, _ := claims["tenant"].(string)
	if fossilTenant == "" {
		fossilTenant = "NULL"
	}

	fossilSubtenant, _ := claims["subtenant"].(string)
	if fossilSubtenant == "" {
		fossilSubtenant = "NULL"
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
	// 🔥 BUILD RLS CLAUSE FROM JWT
	// ==========================
	structuredUsername := "tenant:" + fossilTenant + ":subtenant:" + fossilSubtenant

	log.Println("Generating FOSSIL Guest Token for:", structuredUsername)

	payload := map[string]interface{}{
		"resources": []map[string]interface{}{
			{"type": "dashboard", "id": reqBody.DashboardID},
		},

		// 🔥 FOSSIL NATIVE RLS: Empty
		"rls": []map[string]interface{}{},

		"user": map[string]interface{}{
			"username":   structuredUsername,
			"first_name": "FOSSIL",
			"last_name":  fossilTenant,
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
