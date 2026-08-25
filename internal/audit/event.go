package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type Event struct {
	ID          string          `json:"id"`
	BatchID     string          `json:"batchId"`
	Action      string          `json:"action"`
	Actor       string          `json:"actor"`
	OccurredAt  time.Time       `json:"occurredAt"`
	Revision    int64           `json:"revision"`
	FactSummary json.RawMessage `json:"factSummary"`
	FactDigest  string          `json:"factDigest"`
}

func NewEvent(id, batchID, action, actor string, revision int64, at time.Time, facts any) (Event, error) {
	encoded, err := json.Marshal(facts)
	if err != nil {
		return Event{}, err
	}
	canonical, err := canonicalFacts(encoded)
	if err != nil {
		return Event{}, err
	}
	sum := sha256.Sum256(canonical)
	return Event{ID: id, BatchID: batchID, Action: action, Actor: actor, OccurredAt: at.UTC(), Revision: revision, FactSummary: encoded, FactDigest: hex.EncodeToString(sum[:])}, nil
}

func VerifyEvent(event Event) bool {
	canonical, err := canonicalFacts(event.FactSummary)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(canonical)
	return event.FactDigest == hex.EncodeToString(sum[:])
}

func canonicalFacts(input []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
