package dto

import "time"

// CallOfferRequest places a direct 1:1 call to Number or a WhatsApp JID.
type CallOfferRequest struct {
	Number string `json:"number" validate:"required"`
	Video  bool   `json:"video"`
}

// CallOfferResponse preserves the JSON shape exposed by the current call API.
// The capitalized keys are intentional compatibility keys.
type CallOfferResponse struct {
	ID        string `json:"ID"`
	Recipient string `json:"Recipient"`
}

// CallSessionResponse preserves the safe lifecycle view returned by the call
// API. It deliberately excludes call keys, relay data, and media payloads.
type CallSessionResponse struct {
	ID        string `json:"ID"`
	Peer      string `json:"Peer"`
	Direction string `json:"Direction" enums:"incoming,outgoing"`
	Media     string `json:"Media" enums:"audio,video"`
	State     string `json:"State" enums:"calling,ringing,connecting,active,ended,idle"`
	Reason    string `json:"Reason"`

	CanAnswer       bool `json:"CanAnswer"`
	CanReject       bool `json:"CanReject"`
	CanHangup       bool `json:"CanHangup"`
	PeerVideo       bool `json:"PeerVideo"`
	LocalVideo      bool `json:"LocalVideo"`
	PeerOrientation int  `json:"PeerOrientation"`

	CreatedAt time.Time `json:"CreatedAt"`
	UpdatedAt time.Time `json:"UpdatedAt"`
}

// CallActionResponse is returned after an accepted call control action.
type CallActionResponse struct {
	Status string `json:"status" example:"ok"`
}

// CallVideoRequest enables local camera, accepts a peer upgrade, or sets orientation.
type CallVideoRequest struct {
	Enabled     *bool `json:"enabled"`
	Accept      bool  `json:"accept"`
	Orientation *int  `json:"orientation"`
}
