package persistence

import (
	"encoding/json"
	"time"

	"scenicpermit/internal/audit"
	"scenicpermit/internal/domain"
)

const CurrentSchemaVersion = 1

type database struct {
	SchemaVersion int                                   `json:"schemaVersion"`
	UpdatedAt     time.Time                             `json:"updatedAt"`
	Batches       map[string]json.RawMessage            `json:"batches"`
	Events        map[string][]audit.Event              `json:"events"`
	Idempotency   map[string]map[string]json.RawMessage `json:"idempotency"`
	Permits       []domain.AdmissionPermit              `json:"permits"`
}

func emptyDatabase() database {
	return database{SchemaVersion: CurrentSchemaVersion, Batches: make(map[string]json.RawMessage), Events: make(map[string][]audit.Event), Idempotency: make(map[string]map[string]json.RawMessage), Permits: []domain.AdmissionPermit{}}
}

func cloneDatabase(input database) (database, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return database{}, err
	}
	var output database
	if err := json.Unmarshal(data, &output); err != nil {
		return database{}, err
	}
	return output, nil
}
