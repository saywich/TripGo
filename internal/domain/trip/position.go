package trip

import (
	"math"
	"time"

	"github.com/google/uuid"
)

type Position struct {
	ID          int64
	TripID      ID
	Coordinates Coordinates
	RecordedAt  time.Time
}

func (p Position) Validate(now time.Time) error {
	if !(math.Abs(p.Coordinates.Latitude) <= 90 && math.Abs(p.Coordinates.Longitude) <= 180) {
		return &ValidationError{Field: "coordinates", Message: "latitude or longitude out of range"}
	}

	if p.RecordedAt.IsZero() || p.RecordedAt.After(now.Add(time.Minute)) {
		return &ValidationError{Field: "recorded_at", Message: "must be present and no more than one minute in the future"}
	}

	return nil
}

type StaleTrip struct {
	ID       ID
	DriverID uuid.UUID
	Since    time.Time
}
