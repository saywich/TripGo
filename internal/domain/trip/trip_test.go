package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	trip "github.com/saywich/tripgo/internal/domain/trip"
)

func TestFinish(t *testing.T) {
	for _, status := range []trip.Status{trip.Active, trip.Completed, "unknown"} {
		t.Run(string(status), func(t *testing.T) {
			v := trip.Trip{Status: status}
			at := time.Now()
			err := v.Finish(at)

			if status == trip.Active {
				if err != nil || v.Status != trip.Completed || v.FinishedAt == nil || !v.FinishedAt.Equal(at) {
					t.Fatalf("finish: %+v %v", v, err)
				}
			} else if !errors.Is(err, trip.ErrTripCompleted) || v.Status != status || v.FinishedAt != nil {
				t.Fatalf("invalid transition mutated trip: %+v %v", v, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	valid := trip.Data{DriverID: uuid.New(), UserID: uuid.New()}

	for _, tc := range []struct {
		name   string
		change func(*trip.Data)
		field  string
	}{{"valid", func(*trip.Data) {}, ""}, {"driver", func(d *trip.Data) { d.DriverID = uuid.Nil }, "driver_id"}, {"user", func(d *trip.Data) { d.UserID = uuid.Nil }, "user_id"}, {"price", func(d *trip.Data) { d.Price = -1 }, "price"}, {"latitude", func(d *trip.Data) { d.StartPoint.Latitude = 91 }, "start_point"}, {"longitude", func(d *trip.Data) { d.EndPoint.Longitude = -181 }, "end_point"}} {
		t.Run(tc.name, func(t *testing.T) {
			d := valid
			tc.change(&d)
			err := d.Validate()

			if tc.field == "" {
				if err != nil {
					t.Fatal(err)
				}

				return
			}

			var v *trip.ValidationError

			if !errors.As(err, &v) || v.Field != tc.field {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}
