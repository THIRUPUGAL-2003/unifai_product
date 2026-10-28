package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OutlookConfig holds Microsoft Graph API / Office 365 OAuth credentials.
type OutlookConfig struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
	TenantID     string
	Email        string
	FromName     string
}

type msTokenResponse struct {
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

type msGraphError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// SendOutlook sends an email via Microsoft Graph API OAuth client credentials.
func SendOutlook(cfg OutlookConfig, msg Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return SendOutlookWithContext(ctx, cfg, msg)
}

// SendOutlookWithContext sends an email via Microsoft Graph API.
func SendOutlookWithContext(ctx context.Context, cfg OutlookConfig, msg Message) error {
	if !cfg.Enabled {
		return fmt.Errorf("Outlook / Microsoft 365 is not enabled")
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return fmt.Errorf("Outlook Client ID is required")
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		return fmt.Errorf("Outlook Client Secret (secret ID) is required")
	}
	tenantID := strings.TrimSpace(cfg.TenantID)
	if tenantID == "" {
		return fmt.Errorf("Outlook Tenant ID is required")
	}
	senderEmail := strings.TrimSpace(cfg.Email)
	if senderEmail == "" {
		return fmt.Errorf("Outlook Sender Email is required")
	}
	recipient := mailHeaderLine(msg.To)
	if recipient == "" {
		return fmt.Errorf("recipient email is required")
	}

	// 1. Acquire OAuth2 App Token from Microsoft identity platform (Client Credentials Flow)
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenantID))
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("scope", "https://graph.microsoft.com/.default")
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("microsoft oauth token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var tokErr msTokenResponse
		_ = json.Unmarshal(bodyBytes, &tokErr)
		desc := tokErr.ErrorDesc
		if desc == "" {
			desc = tokErr.Error
		}
		if desc == "" {
			desc = string(bodyBytes)
		}
		return fmt.Errorf("microsoft oauth failed (HTTP %d): %s", resp.StatusCode, desc)
	}

	var tokResp msTokenResponse
	if err := json.Unmarshal(bodyBytes, &tokResp); err != nil {
		return fmt.Errorf("parse oauth token response: %w", err)
	}
	if tokResp.AccessToken == "" {
		return fmt.Errorf("no access token returned by Microsoft identity platform")
	}

	// 2. Dispatch email via Microsoft Graph API sendMail endpoint
	sendMailURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", url.PathEscape(senderEmail))
	payload := map[string]any{
		"message": map[string]any{
			"subject": msg.Subject,
			"body": map[string]any{
				"contentType": "Text",
				"content":     msg.Body,
			},
			"toRecipients": []map[string]any{
				{
					"emailAddress": map[string]any{
						"address": recipient,
					},
				},
			},
		},
		"saveToSentItems": false,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal sendMail payload: %w", err)
	}

	mailReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sendMailURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("create sendMail request: %w", err)
	}
	mailReq.Header.Set("Authorization", "Bearer "+tokResp.AccessToken)
	mailReq.Header.Set("Content-Type", "application/json")

	mailResp, err := client.Do(mailReq)
	if err != nil {
		return fmt.Errorf("microsoft graph sendMail request failed: %w", err)
	}
	defer mailResp.Body.Close()

	if mailResp.StatusCode != http.StatusOK && mailResp.StatusCode != http.StatusAccepted {
		mailErrBytes, _ := io.ReadAll(mailResp.Body)
		var graphErr msGraphError
		_ = json.Unmarshal(mailErrBytes, &graphErr)
		msgStr := graphErr.Error.Message
		if msgStr == "" {
			msgStr = string(mailErrBytes)
		}
		return fmt.Errorf("microsoft graph sendMail failed (HTTP %d): %s", mailResp.StatusCode, msgStr)
	}

	return nil
}
