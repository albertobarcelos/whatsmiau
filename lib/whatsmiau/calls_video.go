package whatsmiau

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/purpshell/meowcaller"
	"go.uber.org/zap"
)

const callVideoMaxAccessUnit = 1 << 20

// CallVideoControl is a JSON control message on the video WebSocket (text frames).
type CallVideoControl struct {
	Type        string `json:"type"`
	PeerVideo   bool   `json:"peerVideo,omitempty"`
	Upgrade     bool   `json:"upgrade,omitempty"`
	Orientation int    `json:"orientation,omitempty"`
}

// CallVideoStream is a bidirectional H.264 Annex-B bridge for a single call.
type CallVideoStream struct {
	Receive <-chan []byte
	Control <-chan CallVideoControl
	Push    func([]byte) error
	Close   func()
}

func attachCallVideo(b *callBridge, call *meowcaller.Call) {
	call.ReceiveVideo(meowcaller.VideoSinkFunc(func(accessUnit []byte) {
		b.video.publish(bytes.Clone(accessUnit))
	}))
	call.OnVideoState(func(state meowcaller.VideoState) {
		if state.Upgrade {
			if err := call.AcceptVideo(); err != nil {
				zap.L().Warn("failed to accept peer video upgrade", zap.String("call_id", call.ID()), zap.Error(err))
			}
		}
		b.update(func(view *CallSession) {
			if state.Active || state.Upgrade || safeCallIsVideo(call) {
				view.Media = "video"
			}
			view.PeerVideo = state.Active
			view.PeerOrientation = state.Orientation
			view.LocalVideo = safeCallIsSendingVideo(call)
		})
		b.controls.publish(CallVideoControl{
			Type:        "state",
			PeerVideo:   state.Active,
			Upgrade:     state.Upgrade,
			Orientation: state.Orientation,
		})
	})
	call.OnVideoKeyframeRequest(func() {
		b.controls.publish(CallVideoControl{Type: "keyframe"})
	})
}

// OpenCallVideo subscribes to peer Annex-B frames and accepts local frames to send.
func (s *Whatsmiau) OpenCallVideo(instanceID, callID string) (*CallVideoStream, error) {
	bridge, err := s.loadCallBridge(instanceID, callID)
	if err != nil {
		return nil, err
	}
	frames, closeFrames := bridge.video.subscribe()
	controls, closeControls := bridge.controls.subscribe()
	return &CallVideoStream{
		Receive: frames,
		Control: controls,
		Push: func(accessUnit []byte) error {
			if !isAnnexB(accessUnit) {
				return fmt.Errorf("invalid h264 annex-b access unit (%d bytes)", len(accessUnit))
			}
			if len(accessUnit) > callVideoMaxAccessUnit {
				return fmt.Errorf("h264 access unit too large: %d", len(accessUnit))
			}
			return bridge.call.SendVideo(bytes.Clone(accessUnit))
		},
		Close: func() {
			closeFrames()
			closeControls()
		},
	}, nil
}

// SetCallVideo enables or stops local camera, accepts a peer upgrade, or sets orientation.
func (s *Whatsmiau) SetCallVideo(instanceID, callID string, enabled *bool, accept bool, orientation *int) error {
	bridge, err := s.loadCallBridge(instanceID, callID)
	if err != nil {
		return err
	}
	if accept {
		if err := bridge.call.AcceptVideo(); err != nil {
			return err
		}
	}
	if enabled != nil {
		if *enabled {
			if !safeCallIsSendingVideo(bridge.call) {
				if err := bridge.call.StartVideo(); err != nil {
					zap.L().Debug("start video", zap.Error(err))
				}
			}
			if err := bridge.call.SetVideoEnabled(true); err != nil {
				return err
			}
		} else if err := bridge.call.StopVideo(); err != nil {
			return err
		}
		bridge.update(func(view *CallSession) {
			view.LocalVideo = *enabled
			if *enabled {
				view.Media = "video"
			}
		})
	}
	if orientation != nil {
		if err := bridge.call.SetVideoOrientation(*orientation); err != nil {
			return err
		}
	}
	return nil
}

func sessionMedia(call *meowcaller.Call) string {
	if safeCallIsVideo(call) {
		return "video"
	}
	return "audio"
}

func safeCallIsVideo(call *meowcaller.Call) (ok bool) {
	if call == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return call.IsVideo()
}

func safeCallIsSendingVideo(call *meowcaller.Call) (ok bool) {
	if call == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return call.IsSendingVideo()
}

func isAnnexB(accessUnit []byte) bool {
	if len(accessUnit) < 4 {
		return false
	}
	if accessUnit[0] == 0 && accessUnit[1] == 0 && accessUnit[2] == 0 && accessUnit[3] == 1 {
		return true
	}
	return accessUnit[0] == 0 && accessUnit[1] == 0 && accessUnit[2] == 1
}

type annexBBroadcaster struct {
	mu          sync.Mutex
	next        uint64
	subscribers map[uint64]chan []byte
	closed      bool
}

func newAnnexBBroadcaster() *annexBBroadcaster {
	return &annexBBroadcaster{subscribers: make(map[uint64]chan []byte)}
}

func (b *annexBBroadcaster) subscribe() (<-chan []byte, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel := make(chan []byte, 2)
	if b.closed {
		close(channel)
		return channel, func() {}
	}
	id := b.next
	b.next++
	b.subscribers[id] = channel
	return channel, func() {
		b.mu.Lock()
		if subscriber, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(subscriber)
		}
		b.mu.Unlock()
	}
}

func (b *annexBBroadcaster) publish(frame []byte) {
	if len(frame) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subscriber := range b.subscribers {
		copyFrame := bytes.Clone(frame)
		select {
		case subscriber <- copyFrame:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- copyFrame:
			default:
			}
		}
	}
}

func (b *annexBBroadcaster) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, subscriber := range b.subscribers {
		delete(b.subscribers, id)
		close(subscriber)
	}
}

type videoControlBroadcaster struct {
	mu          sync.Mutex
	next        uint64
	subscribers map[uint64]chan CallVideoControl
	closed      bool
}

func newVideoControlBroadcaster() *videoControlBroadcaster {
	return &videoControlBroadcaster{subscribers: make(map[uint64]chan CallVideoControl)}
}

func (b *videoControlBroadcaster) subscribe() (<-chan CallVideoControl, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	channel := make(chan CallVideoControl, 8)
	if b.closed {
		close(channel)
		return channel, func() {}
	}
	id := b.next
	b.next++
	b.subscribers[id] = channel
	return channel, func() {
		b.mu.Lock()
		if subscriber, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(subscriber)
		}
		b.mu.Unlock()
	}
}

func (b *videoControlBroadcaster) publish(msg CallVideoControl) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- msg:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- msg:
			default:
			}
		}
	}
}

func (b *videoControlBroadcaster) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, subscriber := range b.subscribers {
		delete(b.subscribers, id)
		close(subscriber)
	}
}
