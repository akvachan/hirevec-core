// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package hirevec

import (
	"encoding/json"
	"net/http"
)

type RequestBodyCreateCandidateReaction struct {
	PositionID   string       `json:"position_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

type RequestBodyCreateCandidate struct {
	About string `json:"about"`
}

type RequestBodyCreateRecruiterReaction struct {
	PositionID   string       `json:"position_id"`
	CandidateID  string       `json:"candidate_id"`
	ReactionType ReactionType `json:"reaction_type"`
}

type RequestBodyCreateMatch struct {
	PositionID  string `json:"position_id"`
	CandidateID string `json:"candidate_id"`
}

type RequestBodyCreateToken struct {
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

func DecodeRequestBody[T any](r *http.Request) (data *T, err error) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err = dec.Decode(data)
	if err != nil {
		return nil, ErrFailedToDecode
	}
	if dec.More() {
		return nil, ErrExtraDataDecoded
	}
	return data, err
}
