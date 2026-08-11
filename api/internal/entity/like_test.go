package entity

import (
	"testing"

	"github.com/google/uuid"
)

func TestMatchPeer(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	if a.String() > b.String() {
		a, b = b, a
	}
	m := Match{AID: a, BID: b}
	if m.Peer(a) != b || m.Peer(b) != a {
		t.Errorf("Peer() mixed up the pair")
	}
	if m.Peer(uuid.New()) != uuid.Nil {
		t.Errorf("Peer() of a stranger must be Nil")
	}
}
