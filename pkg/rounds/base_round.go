package rounds

import (
	"errors"

	"github.com/taurusgroup/frost-ed25519/pkg/frost/party"
)

type BaseRound struct {
	selfID   party.ID
	partySet *party.Set
}

func NewBaseRound(selfID party.ID, partySet *party.Set) (*BaseRound, error) {
	if !partySet.Contains(selfID) {
		return nil, errors.New("partySet should contain selfID")
	}
	return &BaseRound{
		selfID:   selfID,
		partySet: partySet,
	}, nil
}

func (r *BaseRound) SelfID() party.ID {
	return r.selfID
}

func (r *BaseRound) Set() *party.Set {
	return r.partySet
}
