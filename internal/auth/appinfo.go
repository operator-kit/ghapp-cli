package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jferrl/go-githubauth"
)

type appResponse struct {
	Slug string `json:"slug"`
}

type userResponse struct {
	ID int64 `json:"id"`
}

// FetchAppSlug calls GET /app with JWT auth to get the app's slug.
func FetchAppSlug(appID int64, privateKey []byte) (string, error) {
	appTokenSource, err := githubauth.NewApplicationTokenSource(appID, privateKey)
	if err != nil {
		return "", fmt.Errorf("create app token source: %w", err)
	}

	token, err := appTokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("generate JWT: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.github.com/app", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET /app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GET /app: %s: %s", resp.Status, body)
	}

	var app appResponse
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return "", fmt.Errorf("decode /app response: %w", err)
	}

	if app.Slug == "" {
		return "", fmt.Errorf("GET /app: slug is empty")
	}

	return app.Slug, nil
}

// FetchBotUserID calls GET /users/{slug}[bot] (public, no auth) to get the bot's user ID.
func FetchBotUserID(slug string) (int64, error) {
	username := slug + "[bot]"
	apiURL := "https://api.github.com/users/" + url.PathEscape(username)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GET /users/%s: %w", username, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("GET /users/%s: %s: %s", username, resp.Status, body)
	}

	var user userResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return 0, fmt.Errorf("decode /users response: %w", err)
	}

	if user.ID == 0 {
		return 0, fmt.Errorf("GET /users/%s: id is 0", username)
	}

	return user.ID, nil
}
