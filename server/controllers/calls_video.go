package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"
	"github.com/verbeux-ai/whatsmiau/lib/whatsmiau"
	"github.com/verbeux-ai/whatsmiau/server/dto"
	"github.com/verbeux-ai/whatsmiau/utils"
	"go.uber.org/zap"
)

const callVideoMaxBytes = 1 << 20

// SetVideo enables local camera, accepts a peer upgrade, or sets orientation.
// @Summary Enable or accept call video
// @Tags Calls
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param instance path string true "Instance ID"
// @Param callID path string true "Call ID"
// @Param body body dto.CallVideoRequest true "Video control"
// @Success 200 {object} dto.CallActionResponse
// @Failure 401 {object} utils.AuthenticationErrorResponse
// @Failure 404 {object} utils.HTTPErrorResponse
// @Failure 409 {object} utils.HTTPErrorResponse
// @Failure 503 {object} utils.HTTPErrorResponse
// @Router /v1/instance/{instance}/calls/{callID}/video [post]
func (s *Calls) SetVideo(ctx echo.Context) error {
	var request dto.CallVideoRequest
	if err := ctx.Bind(&request); err != nil {
		return utils.HTTPFail(ctx, http.StatusUnprocessableEntity, err, "failed to bind request body")
	}
	err := s.whatsmiau.SetCallVideo(ctx.Param("instance"), ctx.Param("callID"), request.Enabled, request.Accept, request.Orientation)
	if err != nil {
		switch {
		case errors.Is(err, whatsmiau.ErrCallSessionNotFound):
			return utils.HTTPFail(ctx, http.StatusNotFound, err, "call session not found")
		case errors.Is(err, whatsmiau.ErrCallSupportDisabled):
			return utils.HTTPFail(ctx, http.StatusServiceUnavailable, err, "call support is disabled")
		default:
			return utils.HTTPFail(ctx, http.StatusConflict, err, "call video action failed")
		}
	}
	return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Video upgrades to a binary H.264 Annex-B bridge. One WebSocket binary message
// is one access unit. Text frames carry keyframe/state control JSON.
// @Summary Open the call video WebSocket
// @Description Upgrades to WebSocket. Each binary message is one H.264 Annex-B access unit (start code + NALUs). Text JSON {"type":"keyframe"} asks the browser encoder for an IDR. No SDP, relay data, or call keys cross this boundary.
// @Tags Calls
// @Produce application/octet-stream
// @Security ApiKeyAuth
// @Param instance path string true "Instance ID"
// @Param callID path string true "Call ID"
// @Success 101 {string} string "WebSocket Switching Protocols"
// @Failure 401 {object} utils.AuthenticationErrorResponse
// @Failure 404 {object} utils.HTTPErrorResponse
// @Failure 503 {object} utils.HTTPErrorResponse
// @Router /v1/instance/{instance}/calls/{callID}/video [get]
func (s *Calls) Video(ctx echo.Context) error {
	stream, err := s.whatsmiau.OpenCallVideo(ctx.Param("instance"), ctx.Param("callID"))
	if err != nil {
		switch {
		case errors.Is(err, whatsmiau.ErrCallSessionNotFound):
			return utils.HTTPFail(ctx, http.StatusNotFound, err, "call session not found")
		case errors.Is(err, whatsmiau.ErrCallSupportDisabled):
			return utils.HTTPFail(ctx, http.StatusServiceUnavailable, err, "call support is disabled")
		default:
			return utils.HTTPFail(ctx, http.StatusInternalServerError, err, "failed to open call video")
		}
	}
	defer stream.Close()

	conn, err := websocket.Accept(ctx.Response(), ctx.Request(), &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return nil
	}
	defer conn.Close(websocket.StatusNormalClosure, "video bridge closed")
	conn.SetReadLimit(callVideoMaxBytes)

	bridgeContext, cancel := context.WithCancel(ctx.Request().Context())
	defer cancel()
	var fromClient, toClient atomic.Uint64
	defer func() {
		zap.L().Info("call API video bridge closed",
			zap.String("instance", ctx.Param("instance")),
			zap.String("call_id", ctx.Param("callID")),
			zap.Uint64("client_to_call_frames", fromClient.Load()),
			zap.Uint64("call_to_client_frames", toClient.Load()),
		)
	}()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer cancel()
		frames := stream.Receive
		controls := stream.Control
		for frames != nil || controls != nil {
			select {
			case <-bridgeContext.Done():
				return
			case frame, ok := <-frames:
				if !ok {
					frames = nil
					continue
				}
				if err := conn.Write(bridgeContext, websocket.MessageBinary, frame); err != nil {
					return
				}
				toClient.Add(1)
			case control, ok := <-controls:
				if !ok {
					controls = nil
					continue
				}
				payload, marshalErr := json.Marshal(control)
				if marshalErr != nil {
					continue
				}
				if err := conn.Write(bridgeContext, websocket.MessageText, payload); err != nil {
					return
				}
			}
		}
	}()
	for {
		messageType, payload, err := conn.Read(bridgeContext)
		if err != nil {
			break
		}
		switch messageType {
		case websocket.MessageBinary:
			if stream.Push(payload) != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, "expected one valid H.264 Annex-B access unit")
				cancel()
				<-writerDone
				return nil
			}
			fromClient.Add(1)
		case websocket.MessageText:
			var control whatsmiau.CallVideoControl
			if json.Unmarshal(payload, &control) != nil {
				continue
			}
			if control.Type == "orientation" {
				orientation := control.Orientation
				_ = s.whatsmiau.SetCallVideo(ctx.Param("instance"), ctx.Param("callID"), nil, false, &orientation)
			}
		}
	}
	cancel()
	<-writerDone
	return nil
}
