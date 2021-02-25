package rounds

import (
	"errors"
	"fmt"
	"sync"

	"github.com/taurusgroup/frost-ed25519/pkg/messages"
)

type RoundState uint8

const (
	ProcessMessages RoundState = 1 << iota
	ProcessRound
	GenerateMessages
	NextRound

	Finished
	Abort
)

// BaseRound can be seen as the basic state that both protocols should have.
// It provides functionality for handling party IDs, a wrapper for the message queue,
// as well handling the current execution state.
type BaseRound struct {
	// AllPartyIDs is a sorted list of uint32 which represent all parties (including this one)
	// that are participating in the Round
	AllPartyIDs []uint32

	// OtherPartyIDs is a set of IDs from all other parties. It is not ordered, and is mostly used to
	// iterate over the list of IDs.
	OtherPartyIDs map[uint32]bool

	finalError  error
	messages    *messages.Queue
	roundNumber int
	done        chan struct{}
	mtx         sync.Mutex
	selfPartyID uint32
	state       RoundState

	isProcessingStep bool
}

func NewBaseRound(selfPartyID uint32, allPartyIDs []uint32, acceptedTypes []messages.MessageType) (*BaseRound, error) {
	var baseRound BaseRound
	if selfPartyID == 0 {
		return nil, errors.New("selfPartyID cannot be 0")
	}
	baseRound.selfPartyID = selfPartyID

	foundSelfIDInAll := false
	finalAllPartyIDs := make([]uint32, 0, len(allPartyIDs))
	otherPartyIDs := make(map[uint32]bool, len(allPartyIDs))
	for _, id := range allPartyIDs {
		if id == 0 {
			return nil, errors.New("IDs in allPartyIDs cannot be 0")
		}
		if id == selfPartyID && !foundSelfIDInAll {
			finalAllPartyIDs = append(finalAllPartyIDs, id)
			foundSelfIDInAll = true
			continue
		}
		if _, ok := otherPartyIDs[id]; !ok {
			otherPartyIDs[id] = true
			finalAllPartyIDs = append(finalAllPartyIDs, id)
		}
	}
	baseRound.OtherPartyIDs = otherPartyIDs

	if !foundSelfIDInAll {
		return nil, errors.New("selfPartyID must be included in allPartyIDs")
	}
	baseRound.AllPartyIDs = finalAllPartyIDs

	var err error
	baseRound.messages, err = messages.NewMessageQueue(selfPartyID, otherPartyIDs, acceptedTypes)
	if err != nil {
		return nil, err
	}

	baseRound.done = make(chan struct{})

	// The first Round will not have ProcessMessages function, so we give the sentinel to ProcessRound
	baseRound.state = ProcessRound

	return &baseRound, nil
}

// PrepareNextRound checks whether the state of the Round allows us to continue on to the next one.
// If so, then we update the Round number and state, and the caller can then return the next Round.
func (b *BaseRound) PrepareNextRound() bool {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.state == NextRound {
		b.state = ProcessMessages
		b.roundNumber++
		return true
	}
	return false
}

// Abort should be called whenever something bad has happened, where we suspect malicious behaviour.
func (b *BaseRound) Abort(culprit uint32, err error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.state = Abort
	if b.finalError == nil {
		b.finalError = fmt.Errorf("abort: party %d: %w", culprit, err)
		close(b.done)
	} else {
		b.finalError = fmt.Errorf("%v, abort: party %d: %w", b.finalError, culprit, err)
	}
}

// Finish should be called by a defer statement by the last Round of the protocol.
// If an abort happens, then we don't update.
func (b *BaseRound) Finish() {
	if b.state == Abort || b.state == Finished {
		return
	}
	b.state = Finished
	close(b.done)
}

// WaitForFinish blocks until the protocol has finished,
// or until an error is returned.
func (b *BaseRound) WaitForFinish() error {
	<-b.done
	return b.finalError
}

// -----
// Round life cycle
//
// These methods should be called at the beginning of the appropriate Round function,
// accompanied by a defer to NextStep
// -----

func (b *BaseRound) CanProcessMessages() bool {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.isProcessingStep {
		return false
	}

	if b.state == ProcessMessages && b.messages.ReceivedAll() {
		b.isProcessingStep = true
		return true
	}

	return false
}

func (b *BaseRound) CanProcessRound() bool {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.isProcessingStep {
		return false
	}

	if b.state == ProcessRound {
		b.isProcessingStep = true
		return true
	}

	return false
}

func (b *BaseRound) CanGenerateMessages() bool {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if b.isProcessingStep {
		return false
	}

	if b.state == GenerateMessages {
		b.isProcessingStep = true
		return true
	}

	return false
}

// NextStep advances the state, but only if the current state was one of the three above functions
func (b *BaseRound) NextStep() {
	switch b.state {
	case ProcessMessages:
		b.isProcessingStep = false
		b.state <<= 1
		b.messages.NextRound()
	case ProcessRound, GenerateMessages:
		b.isProcessingStep = false
		b.state <<= 1
	}
}

// ----
// Getters
// ----

// ID is the uint32 ID of the party executing this Round.
func (b *BaseRound) ID() uint32 {
	return b.selfPartyID
}

// RoundNumber returns the current Round number
func (b *BaseRound) RoundNumber() int {
	return b.roundNumber
}

// N returns the number of parties participating.
func (b *BaseRound) N() uint32 {
	return uint32(len(b.AllPartyIDs))
}

// ----
// Misc
// ----

// ProcessMessages is implemented here as an empty function so that the BaseRound and subsequent initial Round
// satisfies the Round interface, even when there are no messages to process.
func (b *BaseRound) ProcessMessages() {
}

// -----
// messages.Queue
// -----

// StoreMessage takes in an unmarshalled wire message and attempts to store it in the messages.Queue.
// It returns an error depending on whether the messages.Queue was able to store it.
func (b *BaseRound) StoreMessage(message *messages.Message) error {
	return b.messages.Store(message)
}

// Messages fetches the message from the queue for the current Round.
func (b *BaseRound) Messages() map[uint32]*messages.Message {
	return b.messages.Messages()
}
