// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: Unlicense

// Package hirevec implements core server and client.
package hirevec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type Client struct {
	BaseURL      string
	AccessToken  string
	RefreshToken string
	UserID       ULID
	HTTPClient   *http.Client
}

func NewClient(baseURL string) Client {
	return Client{
		BaseURL:    baseURL,
		HTTPClient: http.DefaultClient,
	}
}

func (c Client) DoRequest(ctx context.Context, method, path string, reqBody any, isJSONAPI bool, res any) error {
	var bodyReader *bytes.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if isJSONAPI {
		req.Header.Set("Accept", JSONAPIMediaType)
	} else {
		req.Header.Set("Accept", "application/json")
	}

	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if res != nil {
		if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("http error %d", resp.StatusCode)
		if isJSONAPI && res != nil {
			if doc, ok := res.(*JSONAPIDocument); ok && len(doc.Errors) > 0 {
				errMsg += fmt.Sprintf(": %s", doc.Errors[0].Detail)
			}
		}
		return fmt.Errorf("request failed: %v", errMsg)
	}

	return nil
}

/*
Health
*/

func (c Client) Health(ctx context.Context) (*JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodGet, string(RouteHealth), nil, true, &doc)
	return &doc, err
}

/*
User management
*/

func (c Client) GetAccessToken(ctx context.Context) (AccessToken, error) {
	payload := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.RefreshToken,
	}
	var token AccessToken
	err := c.DoRequest(ctx, http.MethodPost, string(RouteOAuth2AccessToken), payload, false, &token)
	return token, err
}

// TODO: Enable SSO Registration
// https://github.com/akvachan/hirevec-core/issues/37
func (c Client) Register(ctx context.Context, email, fullName, password string) (TokenPair, error) {
	payload := map[string]string{
		"email":     email,
		"full_name": fullName,
		"password":  password,
	}
	var pair TokenPair
	if err := c.DoRequest(ctx, http.MethodPost, string(RouteV1Me), payload, false, &pair); err != nil {
		return pair, err
	}

	c.UserID = pair.UserID
	return pair, nil
}

// TODO: Enable SSO Login
// https://github.com/akvachan/hirevec-core/issues/37
func (c Client) Login(ctx context.Context, email, password string) (TokenPair, error) {
	payload := map[string]string{
		"email":    email,
		"password": password,
	}
	var pair TokenPair
	if err := c.DoRequest(ctx, http.MethodPost, string(RouteOAuth2Authorize)+"?provider=email", payload, false, &pair); err != nil {
		return pair, err
	}

	c.UserID = pair.UserID
	return pair, nil
}

func (c Client) GetMe(ctx context.Context) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodGet, string(RouteV1Me), nil, true, &doc)
	return doc, err
}

func (c Client) PatchMe(ctx context.Context, userName, fullName *string) (JSONAPIDocument, error) {
	payload := struct {
		UserName *string `json:"user_name,omitempty"`
		FullName *string `json:"full_name,omitempty"`
	}{
		UserName: userName,
		FullName: fullName,
	}
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodPatch, string(RouteV1Me), payload, true, &doc)
	return doc, err
}

func (c Client) DeleteMe(ctx context.Context, password string) (JSONAPIDocument, error) {
	payload := map[string]string{"password": password}
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodDelete, string(RouteV1Me), payload, true, &doc)
	return doc, err
}

/*
Candidate profile management
*/

func (c Client) CreateMeCandidateProfile(ctx context.Context, about string) (AccessToken, error) {
	payload := map[string]string{"about": about}
	var token AccessToken
	err := c.DoRequest(ctx, http.MethodPost, string(RouteV1MeCandidate), payload, false, &token)
	return token, err
}

func (c Client) GetMeCandidateProfile(ctx context.Context) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodGet, string(RouteV1MeCandidate), nil, true, &doc)
	return doc, err
}

func (c Client) PatchMeCandidateProfile(ctx context.Context, about *string) (JSONAPIDocument, error) {
	payload := struct {
		About *string `json:"about,omitempty"`
	}{
		About: about,
	}
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodPatch, string(RouteV1MeCandidate), payload, true, &doc)
	return doc, err
}

func (c Client) DeleteMeCandidateProfile(ctx context.Context) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodDelete, string(RouteV1MeCandidate), nil, true, &doc)
	return doc, err
}

/*
Recruiter profile management
*/

func (c Client) CreateMeRecruiterProfile(ctx context.Context) (AccessToken, error) {
	var token AccessToken
	err := c.DoRequest(ctx, http.MethodPost, string(RouteV1MeRecruiter), nil, false, &token)
	return token, err
}

func (c Client) GetMeRecruiterProfile(ctx context.Context) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodGet, string(RouteV1MeRecruiter), nil, true, &doc)
	return doc, err
}

func (c Client) DeleteMeRecruiterProfile(ctx context.Context) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodDelete, string(RouteV1MeRecruiter), nil, true, &doc)
	return doc, err
}

/*
Positions management
*/

func (c Client) CreateMePosition(ctx context.Context, title, description, company string) (JSONAPIDocument, error) {
	payload := map[string]string{
		"title":       title,
		"description": description,
		"company":     company,
	}
	var doc JSONAPIDocument
	err := c.DoRequest(ctx, http.MethodPost, string(RouteV1MePositions), payload, true, &doc)
	return doc, err
}

func (c Client) GetMePosition(ctx context.Context, id string) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := fmt.Sprintf("/v1/me/positions/%s", id)
	err := c.DoRequest(ctx, http.MethodGet, path, nil, true, &doc)
	return doc, err
}

func (c Client) GetMePositions(ctx context.Context, cursor string, limit int) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := string(RouteV1MePositions)
	query := "?cursor=" + cursor
	if limit > 0 {
		query += "&limit=" + strconv.Itoa(limit)
	}
	err := c.DoRequest(ctx, http.MethodGet, path+query, nil, true, &doc)
	return doc, err
}

func (c Client) PatchMePosition(ctx context.Context, id string, title, description, company *string, isActive *bool) (JSONAPIDocument, error) {
	payload := struct {
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Company     *string `json:"company,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}{
		Title:       title,
		Description: description,
		Company:     company,
		IsActive:    isActive,
	}
	var doc JSONAPIDocument
	path := fmt.Sprintf("/v1/me/positions/%s", id)
	err := c.DoRequest(ctx, http.MethodPatch, path, payload, true, &doc)
	return doc, err
}

func (c Client) DeleteMePosition(ctx context.Context, id string) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := fmt.Sprintf("/v1/me/positions/%s", id)
	err := c.DoRequest(ctx, http.MethodDelete, path, nil, true, &doc)
	return doc, err
}

/*
Recommendations management
*/

func (c Client) GetMeRecommendations(ctx context.Context, posCursor string, canCursor string, limit int, excludeReacted bool) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := fmt.Sprintf("%s?pos_cursor=%s&can_cursor=%s&exclude_reacted=%t", RouteV1MeRecommendations, posCursor, canCursor, excludeReacted)
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	err := c.DoRequest(ctx, http.MethodGet, path, nil, true, &doc)
	return doc, err
}

/*
Reactions management
*/

func (c Client) CreateMeReaction(ctx context.Context, recommendationID string, reactionType ReactionType) (JSONAPIDocument, error) {
	payload := map[string]ReactionType{
		"reaction_type": reactionType,
	}
	var doc JSONAPIDocument
	path := fmt.Sprintf("/v1/me/recommendations/%s/reaction", recommendationID)
	err := c.DoRequest(ctx, http.MethodPost, path, payload, true, &doc)
	return doc, err
}

func (c Client) GetMeReactions(ctx context.Context, cursor string, limit int) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := string(RouteV1MeReactions) + "?cursor=" + cursor
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	err := c.DoRequest(ctx, http.MethodGet, path, nil, true, &doc)
	return doc, err
}

/*
Matches management
*/

func (c Client) GetMeMatches(ctx context.Context, cursor string, limit int) (JSONAPIDocument, error) {
	var doc JSONAPIDocument
	path := string(RouteV1MeMatches) + "?cursor=" + cursor
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	err := c.DoRequest(ctx, http.MethodGet, path, nil, true, &doc)
	return doc, err
}
