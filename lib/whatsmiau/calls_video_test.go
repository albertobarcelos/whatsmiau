package whatsmiau

import (
	"testing"

	"github.com/purpshell/meowcaller"
	"github.com/puzpuzpuz/xsync/v4"
)

func TestIsAnnexB(t *testing.T) {
	if isAnnexB(nil) || isAnnexB([]byte{0, 0, 1}) {
		t.Fatal("short payload must be rejected")
	}
	if !isAnnexB([]byte{0, 0, 0, 1, 0x65}) {
		t.Fatal("4-byte start code must be accepted")
	}
	if !isAnnexB([]byte{0, 0, 1, 0x65, 0}) {
		t.Fatal("3-byte start code must be accepted")
	}
	if isAnnexB([]byte{0, 0, 2, 1, 0x65}) {
		t.Fatal("non-annex-b must be rejected")
	}
}

func TestNewCallBridgeDefaultsToAudioWithoutEngine(t *testing.T) {
	s := &Whatsmiau{callBridges: xsync.NewMap[string, *callBridge]()}
	call := &meowcaller.Call{}
	setCallPhase(t, call, meowcaller.CallPhaseRinging)
	s.trackCallBridge("inst", call, "incoming")
	bridge, ok := s.callBridges.Load(callBridgeKey("inst", call.ID()))
	if !ok {
		t.Fatal("live call bridge missing")
	}
	if got := bridge.snapshot().Media; got != "audio" {
		t.Fatalf("media = %q, want audio when meowcaller engine is absent", got)
	}
	if bridge.video == nil || bridge.controls == nil {
		t.Fatal("video broadcasters must be initialized on every call")
	}
}
