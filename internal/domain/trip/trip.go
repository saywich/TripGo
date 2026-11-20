package trip

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	Active    Status = "active"
	Completed Status = "completed"
)

type ID = uuid.UUID

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

type Coordinates struct {
	Latitude float64 `json:"latitude"`

	Longitude float64 `json:"longitude"`
}

type Trip struct {
	DriverID uuid.UUID `json:"driver_id"`

	EndPoint       Coordinates `json:"end_point"`
	FinishedAt     *time.Time  `json:"finished_at,omitempty"`
	ID             uuid.UUID   `json:"id"`
	LastPositionAt *time.Time  `json:"last_position_at,omitempty"`

	Price int64 `json:"price"`

	StartPoint Coordinates `json:"start_point"`
	StartedAt  time.Time   `json:"started_at"`
	Status     Status      `json:"status"`
	UserID     uuid.UUID   `json:"user_id"`
}

type Data struct {
	DriverID uuid.UUID `json:"driver_id"`

	EndPoint Coordinates `json:"end_point"`

	Price int64 `json:"price"`

	StartPoint Coordinates `json:"start_point"`
	UserID     uuid.UUID   `json:"user_id"`
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != Active {
		return ErrTripCompleted
	}

	t.Status = Completed
	t.FinishedAt = &at

	return nil
}

func (d Data) Validate() error {
	if d.DriverID == uuid.Nil {
		return &ValidationError{"driver_id", "must be a nonzero UUID"}
	}

	if d.UserID == uuid.Nil {
		return &ValidationError{"user_id", "must be a nonzero UUID"}
	}

	if d.Price < 0 {
		return &ValidationError{"price", "must be nonnegative"}
	}

	for _, p := range []struct {
		name string
		c    Coordinates
	}{{"start_point", d.StartPoint}, {"end_point", d.EndPoint}} {
		if math.IsNaN(p.c.Latitude) || math.IsInf(p.c.Latitude, 0) || math.Abs(p.c.Latitude) > 90 || math.IsNaN(p.c.Longitude) || math.IsInf(p.c.Longitude, 0) || math.Abs(p.c.Longitude) > 180 {
			return &ValidationError{p.name, "invalid coordinates"}
		}
	}

	return nil
}
